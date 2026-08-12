// Package store 实现 Cloud 各业务域的 SQLite CRUD（CL-2b）。
//
// 业务对象：User / Plan / Device / Order / Payment / LicenseRow。
// 设计源：docs/07 §8（数据模型）+ docs/05 §8（License）。
//
// V1 简化：
//   - 密码 hash 用 SHA-256（salt+password），不引 bcrypt
//   - features 以 JSON 字符串存
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// 常见错误。
var (
	ErrNotFound        = errors.New("store: not found")
	ErrDuplicate       = errors.New("store: duplicate")
	ErrInvalidStatus   = errors.New("store: invalid status transition")
)

// ----- 业务对象 -----

type User struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	CreatedAt   int64  `json:"created_at"`
}

type Plan struct {
	PlanID       string   `json:"plan_id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	PriceCents   int      `json:"price_cents"`
	Currency     string   `json:"currency"`
	DurationDays int      `json:"duration_days"`
	Features     []string `json:"features"`
	Active       bool     `json:"active"`
}

type Device struct {
	DeviceID    string `json:"device_id"`
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	PairedAt    int64  `json:"paired_at"`
}

type Order struct {
	OrderID     string  `json:"order_id"`
	UserID      string  `json:"user_id"`
	PlanID      string  `json:"plan_id"`
	AmountCents int     `json:"amount_cents"`
	Currency    string  `json:"currency"`
	Status      string  `json:"status"`
	CreatedAt   int64   `json:"created_at"`
	PaidAt      *int64  `json:"paid_at,omitempty"`
}

type LicenseRow struct {
	LicenseID   string   `json:"license_id"`
	UserID      string   `json:"user_id"`
	PlanID      string   `json:"plan_id"`
	OrderID     string   `json:"order_id,omitempty"`
	Status      string   `json:"status"`
	DeviceID    string   `json:"device_id,omitempty"`
	IssuedAt    *int64   `json:"issued_at,omitempty"`
	ValidFrom   *int64   `json:"valid_from,omitempty"`
	ExpiresAt   *int64   `json:"expires_at,omitempty"`
	Features    []string `json:"features"`
	PayloadJSON string   `json:"-"` // 仅 ControlHub 端需要
	Signature   string   `json:"-"`
	CreatedAt   int64    `json:"created_at"`
	ActivatedAt *int64   `json:"activated_at,omitempty"`
}

// ----- Store -----

type Store struct {
	DB *sql.DB
}

func New(db *sql.DB) *Store { return &Store{DB: db} }

// ----- User -----

func (s *Store) CreateUser(email, password string) (User, error) {
	salt := randomHex(16)
	hash := hashPassword(password, salt)
	userID := "acc_" + randomHex(11)
	now := time.Now().Unix()
	_, err := s.DB.Exec(
		`INSERT INTO users(user_id, email, password_hash, password_salt) VALUES(?, ?, ?, ?)`,
		userID, email, hash, salt,
	)
	if err != nil {
		// UNIQUE 冲突判断（modernc sqlite error 含 "UNIQUE constraint"）
		return User{}, fmt.Errorf("%w: %v", ErrDuplicate, err)
	}
	return User{UserID: userID, Email: email, CreatedAt: now}, nil
}

func (s *Store) GetUserByID(userID string) (User, error) {
	var u User
	err := s.DB.QueryRow(
		`SELECT user_id, email, display_name, created_at FROM users WHERE user_id = ?`,
		userID,
	).Scan(&u.UserID, &u.Email, &u.DisplayName, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) VerifyPassword(email, password string) (User, error) {
	var u User
	var hash, salt string
	err := s.DB.QueryRow(
		`SELECT user_id, email, display_name, created_at, password_hash, password_salt
		 FROM users WHERE email = ?`,
		email,
	).Scan(&u.UserID, &u.Email, &u.DisplayName, &u.CreatedAt, &hash, &salt)
	if err == sql.ErrNoRows {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	if hashPassword(password, salt) != hash {
		return User{}, fmt.Errorf("invalid credentials")
	}
	return u, nil
}

// ----- Plan -----

func (s *Store) ListPlans(activeOnly bool) ([]Plan, error) {
	q := `SELECT plan_id, name, description, price_cents, currency, duration_days, features_json, active FROM plans`
	if activeOnly {
		q += ` WHERE active = 1`
	}
	q += ` ORDER BY price_cents ASC`
	rows, err := s.DB.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		var p Plan
		var featuresJSON string
		var active int
		if err := rows.Scan(&p.PlanID, &p.Name, &p.Description, &p.PriceCents,
			&p.Currency, &p.DurationDays, &featuresJSON, &active); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(featuresJSON), &p.Features)
		p.Active = active == 1
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) GetPlan(planID string) (Plan, error) {
	var p Plan
	var featuresJSON string
	var active int
	err := s.DB.QueryRow(
		`SELECT plan_id, name, description, price_cents, currency, duration_days, features_json, active
		 FROM plans WHERE plan_id = ?`,
		planID,
	).Scan(&p.PlanID, &p.Name, &p.Description, &p.PriceCents,
		&p.Currency, &p.DurationDays, &featuresJSON, &active)
	if err == sql.ErrNoRows {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	_ = json.Unmarshal([]byte(featuresJSON), &p.Features)
	p.Active = active == 1
	return p, nil
}

// SeedPlans 用 upsert 插入默认套餐（Cloud 启动时调用一次）。
func (s *Store) SeedPlans(plans []Plan) error {
	for _, p := range plans {
		featuresJSON, _ := json.Marshal(p.Features)
		active := 0
		if p.Active {
			active = 1
		}
		_, err := s.DB.Exec(
			`INSERT INTO plans(plan_id, name, description, price_cents, currency, duration_days, features_json, active)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(plan_id) DO UPDATE SET
			   name=excluded.name, description=excluded.description,
			   price_cents=excluded.price_cents, duration_days=excluded.duration_days,
			   features_json=excluded.features_json, active=excluded.active`,
			p.PlanID, p.Name, p.Description, p.PriceCents, p.Currency,
			p.DurationDays, string(featuresJSON), active,
		)
		if err != nil {
			return fmt.Errorf("seed plan %s: %w", p.PlanID, err)
		}
	}
	return nil
}

// ----- Device -----

func (s *Store) CreateDevice(userID, deviceID, displayName string) error {
	_, err := s.DB.Exec(
		`INSERT INTO devices(device_id, user_id, display_name) VALUES(?, ?, ?)
		 ON CONFLICT(device_id) DO UPDATE SET user_id=excluded.user_id, display_name=excluded.display_name`,
		deviceID, userID, displayName,
	)
	return err
}

func (s *Store) ListDevicesByUser(userID string) ([]Device, error) {
	rows, err := s.DB.Query(
		`SELECT device_id, user_id, display_name, paired_at FROM devices WHERE user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.DeviceID, &d.UserID, &d.DisplayName, &d.PairedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// ----- Order -----

func (s *Store) CreateOrder(userID, planID string, amountCents int) (Order, error) {
	orderID := "ord_" + randomHex(11)
	now := time.Now().Unix()
	_, err := s.DB.Exec(
		`INSERT INTO orders(order_id, user_id, plan_id, amount_cents, status) VALUES(?, ?, ?, ?, 'pending')`,
		orderID, userID, planID, amountCents,
	)
	if err != nil {
		return Order{}, err
	}
	return Order{
		OrderID: orderID, UserID: userID, PlanID: planID,
		AmountCents: amountCents, Currency: "CNY",
		Status: "pending", CreatedAt: now,
	}, nil
}

func (s *Store) GetOrder(orderID string) (Order, error) {
	var o Order
	var paidAt sql.NullInt64
	err := s.DB.QueryRow(
		`SELECT order_id, user_id, plan_id, amount_cents, currency, status, created_at, paid_at
		 FROM orders WHERE order_id = ?`,
		orderID,
	).Scan(&o.OrderID, &o.UserID, &o.PlanID, &o.AmountCents, &o.Currency,
		&o.Status, &o.CreatedAt, &paidAt)
	if err == sql.ErrNoRows {
		return Order{}, ErrNotFound
	}
	if paidAt.Valid {
		v := paidAt.Int64
		o.PaidAt = &v
	}
	return o, err
}

func (s *Store) ListOrdersByUser(userID string) ([]Order, error) {
	rows, err := s.DB.Query(
		`SELECT order_id, user_id, plan_id, amount_cents, currency, status, created_at, paid_at
		 FROM orders WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Order
	for rows.Next() {
		var o Order
		var paidAt sql.NullInt64
		if err := rows.Scan(&o.OrderID, &o.UserID, &o.PlanID, &o.AmountCents,
			&o.Currency, &o.Status, &o.CreatedAt, &paidAt); err != nil {
			return nil, err
		}
		if paidAt.Valid {
			v := paidAt.Int64
			o.PaidAt = &v
		}
		out = append(out, o)
	}
	return out, nil
}

// MarkOrderPaid 在事务里把 order 标记为 paid。
func (s *Store) MarkOrderPaid(orderID string) error {
	now := time.Now().Unix()
	res, err := s.DB.Exec(
		`UPDATE orders SET status = 'paid', paid_at = ? WHERE order_id = ? AND status = 'pending'`,
		now, orderID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrInvalidStatus
	}
	return nil
}

// ----- License -----

// CreateLicense 新建 UNUSED license（订单支付后调用）。
func (s *Store) CreateLicense(userID, planID, orderID string, features []string) (LicenseRow, error) {
	licenseID := "lic_" + randomHex(11)
	featuresJSON, _ := json.Marshal(features)
	_, err := s.DB.Exec(
		`INSERT INTO licenses(license_id, user_id, plan_id, order_id, status, features_json)
		 VALUES(?, ?, ?, ?, 'UNUSED', ?)`,
		licenseID, userID, planID, orderID, string(featuresJSON),
	)
	if err != nil {
		return LicenseRow{}, err
	}
	return LicenseRow{
		LicenseID: licenseID, UserID: userID, PlanID: planID, OrderID: orderID,
		Status: "UNUSED", Features: features,
	}, nil
}

func (s *Store) GetLicense(licenseID string) (LicenseRow, error) {
	var l LicenseRow
	var featuresJSON string
	var orderID, deviceID, payloadJSON, signature sql.NullString
	var issuedAtI, validFromI, expiresAtI, activatedAtI sql.NullInt64
	err := s.DB.QueryRow(
		`SELECT license_id, user_id, plan_id, order_id, status, device_id,
		        issued_at, valid_from, expires_at, features_json, payload_json, signature,
		        created_at, activated_at
		 FROM licenses WHERE license_id = ?`,
		licenseID,
	).Scan(&l.LicenseID, &l.UserID, &l.PlanID, &orderID, &l.Status, &deviceID,
		&issuedAtI, &validFromI, &expiresAtI, &featuresJSON, &payloadJSON, &signature,
		&l.CreatedAt, &activatedAtI)
	if err == sql.ErrNoRows {
		return LicenseRow{}, ErrNotFound
	}
	if err != nil {
		return LicenseRow{}, err
	}
	if orderID.Valid {
		l.OrderID = orderID.String
	}
	if deviceID.Valid {
		l.DeviceID = deviceID.String
	}
	if payloadJSON.Valid {
		l.PayloadJSON = payloadJSON.String
	}
	if signature.Valid {
		l.Signature = signature.String
	}
	if issuedAtI.Valid {
		v := issuedAtI.Int64
		l.IssuedAt = &v
	}
	if validFromI.Valid {
		v := validFromI.Int64
		l.ValidFrom = &v
	}
	if expiresAtI.Valid {
		v := expiresAtI.Int64
		l.ExpiresAt = &v
	}
	if activatedAtI.Valid {
		v := activatedAtI.Int64
		l.ActivatedAt = &v
	}
	_ = json.Unmarshal([]byte(featuresJSON), &l.Features)
	return l, nil
}

func (s *Store) ListLicensesByUser(userID string) ([]LicenseRow, error) {
	rows, err := s.DB.Query(
		`SELECT license_id, user_id, plan_id, order_id, status, device_id,
		        issued_at, valid_from, expires_at, features_json, created_at, activated_at
		 FROM licenses WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LicenseRow
	for rows.Next() {
		var l LicenseRow
		var featuresJSON string
		var orderID, deviceID sql.NullString
		var issuedAt, validFrom, expiresAt, activatedAt sql.NullInt64
		if err := rows.Scan(&l.LicenseID, &l.UserID, &l.PlanID, &orderID, &l.Status, &deviceID,
			&issuedAt, &validFrom, &expiresAt, &featuresJSON, &l.CreatedAt, &activatedAt); err != nil {
			return nil, err
		}
		if orderID.Valid {
			l.OrderID = orderID.String
		}
		if deviceID.Valid {
			l.DeviceID = deviceID.String
		}
		if issuedAt.Valid {
			v := issuedAt.Int64
			l.IssuedAt = &v
		}
		if validFrom.Valid {
			v := validFrom.Int64
			l.ValidFrom = &v
		}
		if expiresAt.Valid {
			v := expiresAt.Int64
			l.ExpiresAt = &v
		}
		if activatedAt.Valid {
			v := activatedAt.Int64
			l.ActivatedAt = &v
		}
		_ = json.Unmarshal([]byte(featuresJSON), &l.Features)
		out = append(out, l)
	}
	return out, nil
}

// ActivateLicense 把 UNUSED License 绑定到 device + 写入签发结果。
func (s *Store) ActivateLicense(licenseID, deviceID string, validFrom, expiresAt int64, payloadJSON, signature string) error {
	now := time.Now().Unix()
	res, err := s.DB.Exec(
		`UPDATE licenses
		 SET status = 'ACTIVE', device_id = ?, issued_at = ?, valid_from = ?, expires_at = ?,
		     payload_json = ?, signature = ?, activated_at = ?
		 WHERE license_id = ? AND status = 'UNUSED'`,
		deviceID, now, validFrom, expiresAt, payloadJSON, signature, now, licenseID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrInvalidStatus
	}
	return nil
}

// ----- 内部辅助 -----

func hashPassword(password, salt string) string {
	sum := sha256.Sum256([]byte(salt + ":" + password))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// 极少失败；fallback 用时间戳保证非空
		return fmt.Sprintf("%0*x", n*2, time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
