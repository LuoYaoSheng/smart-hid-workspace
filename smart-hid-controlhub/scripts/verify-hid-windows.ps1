# verify-hid-windows.ps1 — Smart HID Windows objective verification (self-contained)
#
# What it does (all measurements are system-level, not visual judgment):
#   1. Connects to ControlHub over LAN (Bearer API key, non-loopback path)
#   2. Finds the device, waits for online + usb_hid_ready (boot_id fetched dynamically —
#      moving USB to this PC power-cycles the board, boot_id changes)
#   3. Mouse: cursor baseline -> API move (dx=+300, dy=-150) -> re-measure -> PASS if
#      direction matches and displacement > 50px (Windows pointer acceleration scales
#      relative moves, so exact 1:1 is not expected; do NOT touch the mouse during the run)
#   4. Keyboard: CapsLock baseline -> API tap CAPSLOCK (250ms hold) -> expect flip ->
#      tap again -> expect restore
#   5. Prints PASS/FAIL per test and exits 0 only if all pass
#
# Usage (PowerShell on the Windows PC with the ESP32-S3 plugged into it):
#   powershell -ExecutionPolicy Bypass -File .\verify-hid-windows.ps1 `
#     -HubUrl http://<controlhub-lan-ip>:17890 -ApiKey <your-api-key>
#
# API key lives on the Mac: smart-hid-controlhub/data/initial-api-key.txt

param(
    [Parameter(Mandatory = $true)][string]$HubUrl,
    [Parameter(Mandatory = $true)][string]$ApiKey,
    [string]$DeviceId = "HID-00000001"
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms

function Get-Cursor { [System.Windows.Forms.Cursor]::Position }
function Get-CapsLock { [System.Windows.Forms.Control]::IsKeyLocked([System.Windows.Forms.Keys]::CapsLock) }

$Headers = @{ Authorization = "Bearer $ApiKey" }

function Send-Command([string]$Type, [string]$Action, [hashtable]$Payload, [int]$TtlMs = 5000) {
    $reqId = "win-verify-{0}-{1}" -f (Get-Date -Format 'yyyyMMddHHmmss'), (Get-Random -Maximum 999999)
    $body = @{
        protocol       = "1.0"
        request_id     = $reqId
        device_id      = $DeviceId
        target_boot_id = $script:BootId
        type           = $Type
        action         = $Action
        ttl_ms         = $TtlMs
        payload        = $Payload
    } | ConvertTo-Json -Depth 4
    Invoke-RestMethod -Method Post -Uri "$HubUrl/api/v1/devices/$DeviceId/commands" `
        -Headers $Headers -ContentType 'application/json' -Body $body -TimeoutSec 15
}

$overall = $true

# --- Step 1: find device, wait for online + usb_hid_ready ---
Write-Host "[1] Locating device $DeviceId at $HubUrl ..."
$dev = $null
for ($i = 0; $i -lt 30; $i++) {
    try {
        $list = Invoke-RestMethod -Uri "$HubUrl/api/v1/devices" -Headers $Headers -TimeoutSec 5
        $dev = $list.devices | Where-Object { $_.device_id -eq $DeviceId }
        if ($dev -and $dev.online -and $dev.usb_hid_ready) { break }
    } catch { Start-Sleep -Seconds 2 }
    if ($i -eq 0) { Write-Host "    waiting for device to come online (USB move power-cycles the board, ~20-40s)..." }
    Start-Sleep -Seconds 2
}
if (-not $dev -or -not $dev.online) { Write-Host "FAIL: device not online"; exit 1 }
if (-not $dev.usb_hid_ready) { Write-Host "FAIL: device online but usb_hid_ready=false (is the board plugged into THIS PC?)"; exit 1 }
$script:BootId = $dev.boot_id
Write-Host ("    OK: online={0} usb_hid_ready={1} boot_id={2} firmware={3}" -f $dev.online, $dev.usb_hid_ready, $dev.boot_id, $dev.firmware)

# --- Step 2: mouse move, direction-checked ---
Write-Host "[2] Mouse move test (dx=+300, dy=-150) — DO NOT touch the mouse/keyboard ..."
$before = Get-Cursor
$ack = Send-Command 'mouse' 'move' @{ dx = 300; dy = -150 }
Start-Sleep -Milliseconds 600
$after = Get-Cursor
$dx = $after.X - $before.X; $dy = $after.Y - $before.Y
$mousePass = ($ack.status -eq 'executed') -and ($dx -ge 50) -and ($dy -le -50)
Write-Host ("    ack={0} cursor:({1},{2})->({3},{4}) delta=({5},{6})  {7}" -f $ack.status, $before.X, $before.Y, $after.X, $after.Y, $dx, $dy, ($(if ($mousePass) {'PASS'} else {'FAIL'})))
if (-not $mousePass) { $overall = $false }

# --- Step 3: CapsLock flip + restore ---
Write-Host "[3] CapsLock flip test (tap 250ms x2) ..."
$c0 = Get-CapsLock
$ack = Send-Command 'keyboard' 'tap' @{ key = 'CAPSLOCK'; hold_ms = 250 }
Start-Sleep -Milliseconds 600
$c1 = Get-CapsLock
$ack2 = Send-Command 'keyboard' 'tap' @{ key = 'CAPSLOCK'; hold_ms = 250 }
Start-Sleep -Milliseconds 600
$c2 = Get-CapsLock
$kbdPass = ($ack.status -eq 'executed') -and ($ack2.status -eq 'executed') -and ($c1 -eq (-not $c0)) -and ($c2 -eq $c0)
Write-Host ("    ack={0}/{1} capslock: {2} -> {3} -> {4}  {5}" -f $ack.status, $ack2.status, $c0, $c1, $c2, ($(if ($kbdPass) {'PASS'} else {'FAIL'})))
if (-not $kbdPass) { $overall = $false }

Write-Host ""
if ($overall) { Write-Host "RESULT: ALL PASS — mouse + keyboard paths verified on Windows"; exit 0 }
else { Write-Host "RESULT: FAIL — see markers above"; exit 1 }
