package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"smart-hid-controlhub/internal/command"
)

// handleHealth GET /api/v1/health —— 无鉴权。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"protocol":   command.ProtocolVersion,
		"device_cnt": len(s.devices.List()),
	})
}

// handleDevicesList GET /api/v1/devices
func (s *Server) handleDevicesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	devs := s.devices.List()
	type devOut struct {
		DeviceID    string `json:"device_id"`
		BootID      string `json:"boot_id"`
		Online      bool   `json:"online"`
		USBHIDReady bool   `json:"usb_hid_ready"`
		Firmware    string `json:"firmware"`
	}
	out := make([]devOut, 0, len(devs))
	for _, d := range devs {
		out = append(out, devOut{d.DeviceID, d.BootID, d.Online, d.USBHIDReady, d.Firmware})
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": out, "total": len(out)})
}

// handleDeviceOrCommand 分发 /devices/{id} 与 /devices/{id}/commands。
func (s *Server) handleDeviceOrCommand(w http.ResponseWriter, r *http.Request) {
	// 路径：/api/v1/devices/{id} 或 /api/v1/devices/{id}/commands
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/devices/")
	// rest 形如 {id} 或 {id}/commands
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "missing device_id"})
		return
	}
	deviceID := parts[0]

	if len(parts) == 1 {
		// GET /devices/{id}
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
			return
		}
		d, ok := s.devices.Get(deviceID)
		if !ok {
			writeJSON(w, http.StatusNotFound, errBody{"not_found", "device not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"device_id":     d.DeviceID,
			"boot_id":       d.BootID,
			"online":        d.Online,
			"usb_hid_ready": d.USBHIDReady,
			"firmware":      d.Firmware,
		})
		return
	}

	// parts[1] == "commands"
	if parts[1] == "commands" {
		s.handleSendCommand(w, r, deviceID)
		return
	}
	writeJSON(w, http.StatusNotFound, errBody{"not_found", "unknown path"})
}

// handleSendCommand POST /api/v1/devices/{id}/commands
func (s *Server) handleSendCommand(w http.ResponseWriter, r *http.Request, deviceID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "POST only"})
		return
	}

	var cmd command.SmartHidCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "invalid json: " + err.Error()})
		return
	}

	// 路径 device_id 必须与 body device_id 一致
	if cmd.DeviceID == "" {
		cmd.DeviceID = deviceID
	}
	if cmd.DeviceID != deviceID {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "path device_id != body device_id"})
		return
	}

	ack, terminal, serr := s.engine.Send(r.Context(), &cmd)
	if serr != nil {
		// Kind 决定 HTTP 映射：校验失败 400 / request_id 冲突 409 / 内部错误 500。
		switch serr.Kind {
		case command.ErrKindConflict:
			writeJSON(w, http.StatusConflict, errBody{"request_id_conflict", serr.Message})
		case command.ErrKindInternal:
			s.log.Error("command engine internal error", "request_id", cmd.RequestID, "detail", serr.Message)
			writeJSON(w, http.StatusInternalServerError, errBody{"internal", "command engine error"})
		default:
			fields := make([]map[string]string, 0, len(serr.Fields))
			for _, e := range serr.Fields {
				fields = append(fields, map[string]string{"field": e.Field, "message": e.Message})
			}
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error":     "validation_failed",
				"device_id": cmd.DeviceID,
				"fields":    fields,
			})
		}
		return
	}

	// 未收到终态 ack → 202 Accepted
	if !terminal {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"request_id": cmd.RequestID,
			"device_id":  cmd.DeviceID,
			"status":     "accepted_not_acked",
			"message":    "command published but no terminal ack within ttl_ms",
		})
		return
	}

	// 终态映射 HTTP
	switch ack.Status {
	case command.AckExecuted:
		writeJSON(w, http.StatusOK, ack)
	case command.AckDuplicate:
		writeJSON(w, http.StatusOK, ack) // 200 + status=duplicate（幂等）
	case command.AckRejected:
		writeJSON(w, http.StatusUnprocessableEntity, ack)
	case command.AckExpired:
		writeJSON(w, http.StatusGatewayTimeout, ack)
	default:
		writeJSON(w, http.StatusOK, ack)
	}
}

// handleCommandQuery GET /api/v1/commands/{request_id}
func (s *Server) handleCommandQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errBody{"method_not_allowed", "GET only"})
		return
	}
	requestID := strings.TrimPrefix(r.URL.Path, "/api/v1/commands/")
	if requestID == "" {
		writeJSON(w, http.StatusBadRequest, errBody{"bad_request", "missing request_id"})
		return
	}
	status, code, execMs, found, _ := s.engine.QueryCommand(requestID)
	if !found {
		writeJSON(w, http.StatusNotFound, errBody{"not_found", "command not found"})
		return
	}
	resp := map[string]any{
		"request_id": requestID,
		"status":     status,
		"code":       code,
	}
	if execMs.Valid {
		resp["execution_ms"] = execMs.Int
	}
	writeJSON(w, http.StatusOK, resp)
}
