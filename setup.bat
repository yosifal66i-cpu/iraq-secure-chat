@echo off
title IraqSecureChat - Auto Installer
color 0B

echo ============================================
echo    IraqSecureChat - التثبيت التلقائي
echo    منصة المراسلة الآمنة للجهات الحكومية
echo ============================================
echo.

:: Run as admin
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo [!] تشغيل بصلاحيات المسؤول...
    powershell start-process -verb runas '%0'
    exit /b
)

echo [1/5] جاري تحميل IraqSecureChat...
powershell -Command "& {Invoke-WebRequest -Uri 'https://github.com/yosifal66i-cpu/iraq-secure-chat/archive/refs/heads/main.zip' -OutFile '%TEMP%\iraqchat.zip'}" >nul 2>&1
powershell -Command "& {Expand-Archive -Path '%TEMP%\iraqchat.zip' -DestinationPath '%USERPROFILE%\Desktop\IraqSecureChat' -Force}" >nul 2>&1
echo [✓] تم التحميل إلى سطح المكتب
echo.

echo [2/5] تجهيز البيئة...
cd /d "%USERPROFILE%\Desktop\IraqSecureChat\iraq-secure-chat-main"
echo [✓] جاهز
echo.

echo [3/5] تثبيت المتطلبات وتشغيل الموقع...
echo قد تستغرق هذه العملية دقيقة...
docker-compose -f infra/docker-compose.yml up -d >nul 2>&1
if %errorLevel% equ 0 (
    echo [✓] تم تشغيل جميع الخدمات
) else (
    echo [!] Docker غير متوفر - سيتم تشغيل الواجهة فقط
    cd web
    call npm install >nul 2>&1
    start /B cmd /c "npm run dev"
    cd ..
)
echo.

echo [4/5] إنشاء اختصار سطح المكتب...
echo [InternetShortcut] > "%USERPROFILE%\Desktop\IraqSecureChat.url"
echo URL=http://localhost:3000 >> "%USERPROFILE%\Desktop\IraqSecureChat.url"
echo [✓] تم إنشاء الاختصار
echo.

echo [5/5] فتح الموقع...
start http://localhost:3000
echo.

echo ============================================
echo    IraqSecureChat جاهز للاستخدام!
echo ============================================
echo.
echo    الموقع: http://localhost:3000
echo    API:    http://localhost:8090
echo    المسار: %USERPROFILE%\Desktop\IraqSecureChat
echo.
echo ============================================
pause
