# Mesh-CU Flutter Web Interface

Flutter web application for Mesh-CU messenger - a distributed peer-to-peer messaging system.

## Features

- Real-time messaging via WebSocket
- Chat list with unread message indicators
- Direct and group chats support
- Peer discovery display
- Material Design 3 UI

## Project Structure

```
flutter_app/
├── lib/
│   ├── main.dart              # App entry point
│   ├── models/
│   │   └── models.dart        # Data models (Peer, Chat, Message)
│   ├── services/
│   │   └── mesh_service.dart  # WebSocket service & state management
│   ├── screens/
│   │   └── home_screen.dart   # Main screen layout
│   └── widgets/
│       ├── chat_list.dart     # Chat list component
│       ├── message_list.dart  # Messages view & input
│       └── peer_list.dart     # Active peers display
├── web/
│   └── index.html             # Web entry point
└── pubspec.yaml               # Dependencies
```

## Setup

### Prerequisites

1. Install Flutter SDK (3.0+)
2. Enable web support: `flutter config --enable-web`

### Installation

```bash
cd flutter_app
flutter pub get
```

### Running

**Web:**
```bash
flutter run -d chrome
```

**Linux:**
```bash
flutter run -d linux
```

**Build for Web:**
```bash
flutter build web
```

## Connecting to Backend

The app connects to the Go backend via WebSocket. Default settings:
- Host: `localhost`
- Port: `8765`

To change the connection settings, use the connection panel at the top of the app.

## Backend Integration

The Flutter app communicates with the Go backend through WebSocket API (`/ws` endpoint).

### WebSocket Messages

**Client → Server:**
```json
{
  "type": "send_message",
  "payload": {
    "chat_id": "ALL",
    "content": "Hello!",
    "recipient_id": "ALL"
  }
}
```

**Server → Client:**
```json
{
  "type": "new_message",
  "payload": {
    "id": 1,
    "chat_id": "ALL",
    "sender_id": "node-123",
    "sender_name": "Alice",
    "content": "Hello!",
    "timestamp": 1234567890,
    "is_read": true
  }
}
```

### Message Types

| Type | Direction | Description |
|------|-----------|-------------|
| `send_message` | C→S | Send a chat message |
| `get_messages` | C→S | Request messages for a chat |
| `get_chats` | C→S | Refresh chat list |
| `get_peers` | C→S | Refresh peer list |
| `create_group` | C→S | Create a new group |
| `new_message` | S→C | New message received |
| `chats` | S→C | Chat list update |
| `peers` | S→C | Peer list update |
| `messages` | S→C | Messages for a chat |
| `error` | S→C | Error response |

## Architecture

- **Provider** for state management
- **WebSocket Channel** for real-time communication
- **Material Design 3** for UI components
- **ChangeNotifier** pattern for reactive updates
