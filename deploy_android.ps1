# Скрипт для сборки и отправки ноды на Android (Termux)

# 1. Задаем параметры кросс-компиляции под Android ARM64
$env:GOOS="android"
$env:GOARCH="arm64"
$env:CGO_ENABLED="0"

# Имя исполняемого файла
$BinaryName = "mesh_android"

# 2. Компилируем (флаги -s -w срезают отладочный мусор и уменьшают размер)
go build -ldflags="-s -w" -o ./$BinaryName ./cmd/node/main.go

# 3. Сразу сбрасываем переменные окружения на компе, чтобы не сломать обычную сборку
$env:GOOS=""
$env:GOARCH=""
$env:CGO_ENABLED=""

# 4. Данные для подключения к телефону (поменяй на свой IP и юзернейм из whoami)
$SSH_User = "u0_a243"
$SSH_IP = "192.168.1.235"
$SSH_Port = "8022"


# 5. Закидываем по SSH сразу в корень Термукса
scp -P $SSH_Port ./$BinaryName "${SSH_User}@${SSH_IP}:~/"


## как рабоает:
## термукс: sshd, whomai
## терминал: ssh u0_a243@192.168.1.235 -p 8022