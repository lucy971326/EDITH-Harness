; 基于 Wails beta.26 的 project.nsi / wails_tools.nsh，许可证见 WAILS-LICENSE.txt。
; 保留官方 MUI、架构判断、WebView2 引导方案，只支持当前用户。
Unicode true
ManifestDPIAware true
RequestExecutionLevel user
SetCompressor /SOLID lzma
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
!include "WinVer.nsh"
!include "WordFunc.nsh"
!include "Sections.nsh"

!define APP_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.edith.harness.desktop"
!define WEBVIEW_KEY "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"
Name "EDITH"
OutFile "${OUTPUT}"
InstallDir "$LOCALAPPDATA\Programs\EDITH"
VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=2052 "ProductName" "EDITH"
VIAddVersionKey /LANG=2052 "FileDescription" "EDITH 安装程序"
VIAddVersionKey /LANG=2052 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=2052 "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=2052 "LegalCopyright" "EDITH"
!define MUI_ICON "icon.ico"
!define MUI_UNICON "icon.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_FUNCTION Launch
!define MUI_FINISHPAGE_RUN_TEXT "启动 EDITH"
!define MUI_FINISHPAGE_RUN_NOTCHECKED
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH
!insertmacro MUI_LANGUAGE "SimpChinese"

Var AppMutex

; 与 cmd/harness-desktop/main.go 的 UniqueID 和 Wails Windows mutex 命名保持一致。
; 持有到安装器退出，防止检查通过后应用又启动；不结束用户的任务。
!macro Guard PREFIX
Function ${PREFIX}Guard
  retry:
  System::Call 'kernel32::CreateMutexW(p 0, i 0, w "wails-app-com.edith.harness.desktop-sim") p.r0 ?e'
  Pop $1
  ${If} $0 == 0
  ${OrIf} $1 == 183
    ${If} $0 != 0
      System::Call 'kernel32::CloseHandle(p r0)'
    ${EndIf}
    IfSilent blocked
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "请先从托盘退出 EDITH，并关闭其他安装或卸载窗口，然后重试。" IDRETRY retry
    blocked:
    SetErrorLevel 10
    Quit
  ${EndIf}
  StrCpy $AppMutex $0
FunctionEnd
!macroend
!insertmacro Guard ""
!insertmacro Guard "un."

Function ReadWebView
  ; Microsoft 的 Evergreen 运行时登记在 32 位注册表视图。
  SetRegView 32
  ReadRegStr $0 HKLM "${WEBVIEW_KEY}" "pv"
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    ReadRegStr $0 HKCU "${WEBVIEW_KEY}" "pv"
  ${EndIf}
  ${If} $0 == "0.0.0.0"
    StrCpy $0 ""
  ${EndIf}
  SetRegView 64
FunctionEnd

Section "EDITH（必需）" Main
  SectionIn RO
  Call ReadWebView
  ${If} $0 == ""
    InitPluginsDir
    SetOutPath "$PLUGINSDIR"
    File /oname=WebView2Setup.exe "${BOOTSTRAPPER}"
    DetailPrint "正在安装 Microsoft Edge WebView2…"
    ClearErrors
    ExecWait '"$PLUGINSDIR\WebView2Setup.exe" /silent /install' $1
    ${If} ${Errors}
      StrCpy $1 "无法启动"
      Goto webviewFailed
    ${EndIf}
    ${If} $1 != 0
    ${AndIf} $1 != 3010
      Goto webviewFailed
    ${EndIf}
    Call ReadWebView
    ${If} $0 == ""
      Goto webviewFailed
    ${EndIf}
  ${EndIf}

  ; 先完整解压新文件，再切换；旧程序移动失败时保留原文件。
  ClearErrors
  SetOutPath "$INSTDIR"
  IfErrors installFailed
  File /oname=EDITH.exe.new "${BINARY}"
  IfErrors installFailed
  IfFileExists "$INSTDIR\EDITH.exe" 0 replace
  Delete "$INSTDIR\EDITH.exe.previous"
  IfErrors installFailed
  Rename "$INSTDIR\EDITH.exe" "$INSTDIR\EDITH.exe.previous"
  IfErrors installFailed
  replace:
  ClearErrors
  Rename "$INSTDIR\EDITH.exe.new" "$INSTDIR\EDITH.exe"
  ${If} ${Errors}
    Rename "$INSTDIR\EDITH.exe.previous" "$INSTDIR\EDITH.exe"
    Goto installFailed
  ${EndIf}
  Delete "$INSTDIR\EDITH.exe.previous"
  ClearErrors
  WriteUninstaller "$INSTDIR\uninstall.exe"
  CreateShortcut "$SMPROGRAMS\EDITH.lnk" "$INSTDIR\EDITH.exe"
  WriteRegStr HKCU "${APP_KEY}" "DisplayName" "EDITH"
  WriteRegStr HKCU "${APP_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${APP_KEY}" "DisplayIcon" "$INSTDIR\EDITH.exe"
  WriteRegStr HKCU "${APP_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${APP_KEY}" "UninstallString" '$\"$INSTDIR\uninstall.exe$\"'
  WriteRegStr HKCU "${APP_KEY}" "QuietUninstallString" '$\"$INSTDIR\uninstall.exe$\" /S'
  WriteRegDWORD HKCU "${APP_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${APP_KEY}" "NoRepair" 1
  IfErrors installFailed
  ; 升级时取消桌面入口，只删除此前由安装器创建的快捷方式。
  ReadRegDWORD $0 HKCU "${APP_KEY}" "DesktopShortcut"
  ${If} $0 == 1
    Delete "$DESKTOP\EDITH.lnk"
  ${EndIf}
  WriteRegDWORD HKCU "${APP_KEY}" "DesktopShortcut" 0
  Goto done
  webviewFailed:
    MessageBox MB_OK|MB_ICONSTOP "WebView2 安装失败（$1）。请检查网络或手动安装 WebView2 后重试。" /SD IDOK
    SetErrorLevel 20
    Abort
  installFailed:
    MessageBox MB_OK|MB_ICONSTOP "无法完成安装。请检查文件占用、目录权限与磁盘空间后重试。" /SD IDOK
    SetErrorLevel 30
    Abort
  done:
SectionEnd

Section /o "桌面快捷方式" DesktopShortcut
  ClearErrors
  CreateShortcut "$DESKTOP\EDITH.lnk" "$INSTDIR\EDITH.exe"
  WriteRegDWORD HKCU "${APP_KEY}" "DesktopShortcut" 1
  ${If} ${Errors}
    SetErrorLevel 30
    Abort "无法创建桌面快捷方式。"
  ${EndIf}
SectionEnd

Function .onInit
  SetShellVarContext current
  SetRegView 64
  ; 固定当前用户的安装位置，忽略 /D，避免升级到另一个目录。
  StrCpy $INSTDIR "$LOCALAPPDATA\Programs\EDITH"
  ${IfNot} ${AtLeastWin10}
    MessageBox MB_OK|MB_ICONSTOP "EDITH 需要 Windows 10 或更新版本。" /SD IDOK
    SetErrorLevel 64
    Quit
  ${EndIf}
  !if "${ARCH}" == "amd64"
    ${IfNot} ${IsNativeAMD64}
  !else if "${ARCH}" == "arm64"
    ${IfNot} ${IsNativeARM64}
  !else
    !error "不支持的安装包架构"
  !endif
      MessageBox MB_OK|MB_ICONSTOP "请下载适合当前电脑架构的 EDITH 安装包。" /SD IDOK
      SetErrorLevel 65
      Quit
    ${EndIf}
  Call Guard
  ; 上次若在切换文件时被中断，先恢复可运行程序再重试安装。
  IfFileExists "$INSTDIR\EDITH.exe" recovered
  IfFileExists "$INSTDIR\EDITH.exe.previous" 0 recovered
  Rename "$INSTDIR\EDITH.exe.previous" "$INSTDIR\EDITH.exe"
  recovered:
  ReadRegStr $0 HKCU "${APP_KEY}" "DisplayVersion"
  ${If} $0 != ""
    ${VersionCompare} $0 "${VERSION}" $1
    ${If} $1 == 1
      MessageBox MB_OK|MB_ICONSTOP "已安装更高版本 EDITH（$0），不能降级安装 ${VERSION}。" /SD IDOK
      SetErrorLevel 11
      Quit
    ${EndIf}
  ${EndIf}
  ReadRegDWORD $0 HKCU "${APP_KEY}" "DesktopShortcut"
  ${If} $0 == 1
    !insertmacro SelectSection ${DesktopShortcut}
  ${EndIf}
FunctionEnd

Function Launch
  System::Call 'kernel32::CloseHandle(p $AppMutex)'
  ExecShell "open" "$INSTDIR\EDITH.exe"
FunctionEnd

Function un.onInit
  SetShellVarContext current
  SetRegView 64
  StrCpy $INSTDIR "$LOCALAPPDATA\Programs\EDITH"
  Call un.Guard
FunctionEnd

Section "Uninstall"
  ClearErrors
  Delete "$INSTDIR\EDITH.exe"
  IfErrors uninstallFailed
  Delete "$INSTDIR\EDITH.exe.new"
  Delete "$INSTDIR\EDITH.exe.previous"
  Delete "$INSTDIR\uninstall.exe"
  Delete "$SMPROGRAMS\EDITH.lnk"
  ReadRegDWORD $0 HKCU "${APP_KEY}" "DesktopShortcut"
  ${If} $0 == 1
    Delete "$DESKTOP\EDITH.lnk"
  ${EndIf}
  IfErrors uninstallFailed
  DeleteRegKey HKCU "${APP_KEY}"
  ; 非递归删除：用户放入的其他文件及 ~/.harness 均保留。
  RMDir "$INSTDIR"
  Goto done
  uninstallFailed:
    MessageBox MB_OK|MB_ICONSTOP "文件无法删除，请检查 EDITH 是否完全退出、目录是否可写，然后重试卸载。" /SD IDOK
    SetErrorLevel 30
    Abort
  done:
SectionEnd
