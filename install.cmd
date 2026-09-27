@echo off
rem jevcli installer (Windows): downloads the release binary, then `jevcli install` puts the agent skill in
rem %USERPROFILE%\.claude\skills, %USERPROFILE%\.agents\skills (and .codex if present) and adds the Claude Code hook
rem template (disabled).
rem   curl -fsSLo install.cmd https://muthuishere.github.io/jevcli/install.cmd && install.cmd
rem   set JEVCLI_VERSION=v0.1.0 / set JEVCLI_NO_HOOK=1 before running to pin a version / install skills only
setlocal
set REPO=muthuishere/jevcli
if "%JEVCLI_BIN%"=="" set JEVCLI_BIN=%LOCALAPPDATA%\Programs\jevcli
if "%JEVCLI_VERSION%"=="" set JEVCLI_VERSION=latest
set ARCH=amd64
if /I "%PROCESSOR_ARCHITECTURE%"=="ARM64" set ARCH=arm64
set ASSET=jevcli_windows_%ARCH%.exe
if "%JEVCLI_VERSION%"=="latest" (set URL=https://github.com/%REPO%/releases/latest/download/%ASSET%) else (set URL=https://github.com/%REPO%/releases/download/%JEVCLI_VERSION%/%ASSET%)
if not exist "%JEVCLI_BIN%" mkdir "%JEVCLI_BIN%"
echo jevcli: downloading %ASSET% (%JEVCLI_VERSION%)
curl -fsSL "%URL%" -o "%JEVCLI_BIN%\jevcli.exe"
if errorlevel 1 (
  where gh >nul 2>nul || (echo jevcli: download failed: %URL% & exit /b 1)
  if "%JEVCLI_VERSION%"=="latest" (gh release download -R %REPO% -p %ASSET% -D "%TEMP%" --clobber) else (gh release download %JEVCLI_VERSION% -R %REPO% -p %ASSET% -D "%TEMP%" --clobber)
  if errorlevel 1 exit /b 1
  move /Y "%TEMP%\%ASSET%" "%JEVCLI_BIN%\jevcli.exe" >nul
)
"%JEVCLI_BIN%\jevcli.exe" version
if "%JEVCLI_NO_HOOK%"=="" ("%JEVCLI_BIN%\jevcli.exe" install) else ("%JEVCLI_BIN%\jevcli.exe" install --skills)
echo %PATH% | find /I "%JEVCLI_BIN%" >nul || (
  powershell -NoProfile -Command "$p=[Environment]::GetEnvironmentVariable('Path','User'); [Environment]::SetEnvironmentVariable('Path', ($p.TrimEnd(';')+';%JEVCLI_BIN%').TrimStart(';'), 'User')"
  echo jevcli: added %JEVCLI_BIN% to your user PATH; open a new terminal
)
endlocal
