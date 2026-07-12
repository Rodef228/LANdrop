# Скрипт для сборки и деплоя ноды под Windows

# 1. Сбрасываем переменные кросс-компиляции, чтобы собирать под родную систему
$env:GOOS="windows"
$env:GOARCH="amd64"
$env:CGO_ENABLED="0"

# Название выходного файла и папка назначения (куда копировать собранный бинарник)
$BinaryName = "mesh_win.exe"


# 3. Компилируем под Windows (флаги -s -w убирают отладочную инфу и уменьшают exe)
go build -ldflags="-s -w" -o "${BinaryName}" ./cmd/node/main.go

# 4. Сбрасываем переменные окружения
$env:GOOS=""
$env:GOARCH=""
$env:CGO_ENABLED=""
