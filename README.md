# Mesh-CU Messenger

Распределённая P2P мессенджер система с поддержкой веб-интерфейса на Flutter.

## Запуск

### Backend (Go node)

```bash
go run ./cmd/node/ -name [your name] -port 8080
```
### Флаги:
* `-name` - имя пользователя (если не указывать, то генерится само)
* `-port` - порт (по умолчанию: 8080)
* `-storage` - путь до папки, сохранение

После запуска backend также стартует WebSocket API сервер на порту **8765** для подключения веб-клиентов.

### Frontend (Flutter Web)

```bash
cd flutter_app
flutter pub get
flutter run -d chrome
```

Или откройте `http://localhost:8765` в браузере после сборки.

## Управление (CLI)

* `/chat [nick-name]` - открыть чат/группу с определенным человеком
* `/new messages` - список чатов/групп c непрочитанными сообщениями
* `/all` - список всех чатов/групп
* `/create -name [group name] -parts [nick1, nick2, ...]` - создать группу с участниками
* `/help` - показать справку

## Архитектура

- **Backend (Go)**: TCP server для P2P сообщений, UDP multicast для discovery, SQLite для хранения
- **API**: WebSocket сервер для подключения веб-клиентов
- **Frontend (Flutter)**: Веб-интерфейс с реальным временем обновлений

## Структура проекта

```
.
├── cmd/node/           # Точка входа Go приложения
├── internal/
│   ├── api/           # WebSocket API для Flutter
│   ├── db/            # SQLite база данных
│   ├── discovery/     # Peer discovery через UDP multicast
│   ├── network/       # TCP сервер для P2P сообщений
│   └── protocol/      # Формат сообщений
├── flutter_app/       # Flutter веб-приложение
│   ├── lib/
│   │   ├── models/    # Модели данных
│   │   ├── services/  # WebSocket сервис
│   │   ├── screens/   # UI экраны
│   │   └── widgets/   # UI компоненты
│   └── web/           # Web entry point
└── README.md
```

## Протокол

Сообщения передаются в формате JSON через TCP/WebSocket:

```json
{
  "type": "CHAT",
  "sender_id": "node-123",
  "sender_name": "Alice",
  "recipient_id": "ALL",
  "payload": {
    "message": "Hello!",
    "sender_name": "Alice"
  }
}
```

### Типы сообщений

- `PING`/`PONG` - проверка соединения
- `CHAT` - текстовое сообщение
- `GROUP_CREATE` - создание группы
- `FILE_REQ`/`FILE_CHUNK`/`FILE_ACK` - передача файлов
