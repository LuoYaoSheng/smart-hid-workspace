; Smart HID ControlHub NSIS installer script (CH-P8).
; 用法：makensis controlhub.nsi
; 前置：packaging/ControlHub.exe 与 packaging/ControlHub.exe.manifest 已由
;       packaging/build-windows.sh 生成。
;
; 设计源：docs/05 §1（双击可运行，无黑窗，托盘）+ §10 验收 A1/A2 +
;         §4 端口（17891 MQTT / 17892 Pairing 入站防火墙放行）。
;
; 安装目录：%LOCALAPPDATA%\SmartHID\ControlHub（与 docs/05 §10 一致）
; 不做：Windows Service（docs/05 §1 明确 V1 不做）、开机自启（tray 菜单后续提供）

!define APPNAME "Smart HID ControlHub"
!define APPVERSION "1.0.0"
!define COMPANY "Smart HID"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\SmartHIDControlHub"

Name "${APPNAME}"
OutFile "ControlHub_Setup.exe"
Unicode true
ShowInstDetails show
ShowUnInstDetails show
RequestExecutionLevel admin ; 防火墙规则需要 admin

InstallDir "$LOCALAPPDATA\SmartHID\ControlHub"
InstallDirRegKey HKCU "${UNINSTALL_KEY}" "InstallLocation"

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

Section "Install" SecInstall
  SetOutPath "$INSTDIR"
  File "ControlHub.exe"
  File "ControlHub.exe.manifest"

  ; 开始菜单快捷方式
  CreateDirectory "$SMPROGRAMS\Smart HID"
  CreateShortcut "$SMPROGRAMS\Smart HID\ControlHub.lnk" "$INSTDIR\ControlHub.exe"

  ; 数据目录（ControlHub 运行时存 controlhub.db / initial-api-key.txt 等）
  CreateDirectory "$INSTDIR\data"
  CreateDirectory "$INSTDIR\logs"

  ; 入站防火墙规则：MQTT 17891 + Pairing 17892（局域网设备需要）
  DetailPrint "Adding firewall rules..."
  nsExec::ExecToLog 'netsh advfirewall firewall add rule name="Smart HID MQTT" dir=in action=allow protocol=TCP localport=17891'
  nsExec::ExecToLog 'netsh advfirewall firewall add rule name="Smart HID Pairing" dir=in action=allow protocol=TCP localport=17892'

  ; 注册卸载信息（控制面板可见）
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "${APPNAME}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${APPVERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "${COMPANY}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" "$\"$INSTDIR\Uninstall.exe$\""
  WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd

Section "Uninstall" SecUninstall
  ; 先删防火墙
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="Smart HID MQTT"'
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="Smart HID Pairing"'

  Delete "$INSTDIR\ControlHub.exe"
  Delete "$INSTDIR\ControlHub.exe.manifest"
  Delete "$INSTDIR\Uninstall.exe"
  Delete "$SMPROGRAMS\Smart HID\ControlHub.lnk"
  RMDir "$SMPROGRAMS\Smart HID"

  ; 数据目录：默认保留（用户可能想保留 controlhub.db 的 trial 用量与配对凭据）
  ; 用户想完全清理时手动删除 $INSTDIR
  DeleteRegKey HKCU "${UNINSTALL_KEY}"

  ; 提示：数据目录保留
  MessageBox MB_ICONINFORMATION "ControlHub 已卸载。$\n$\n数据目录保留在 $INSTDIR（含 trial 用量、配对凭据）。$\n如需完全删除请手动清理。"
SectionEnd
