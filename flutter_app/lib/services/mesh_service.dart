import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:web_socket_channel/web_socket_channel.dart';
import '../models/models.dart';

class MeshService extends ChangeNotifier {
  WebSocketChannel? _channel;
  bool _isConnected = false;
  String _serverAddress = 'localhost';
  int _serverPort = 8765;

  List<Peer> _peers = [];
  List<Chat> _chats = [];
  Map<String, List<Message>> _messages = {};
  Chat? _selectedChat;
  String _nodeId = '';

  bool get isConnected => _isConnected;
  List<Peer> get peers => _peers;
  List<Chat> get chats => _chats;
  Chat? get selectedChat => _selectedChat;
  List<Message> get messages => _selectedChat != null 
    ? (_messages[_selectedChat!.id] ?? []) 
    : [];
  String get nodeId => _nodeId;

  void setServerAddress(String address, int port) {
    _serverAddress = address;
    _serverPort = port;
    notifyListeners();
  }

  Future<void> connect() async {
    try {
      final uri = Uri.parse('ws://$_serverAddress:$_serverPort/ws');
      _channel = WebSocketChannel.connect(uri);
      
      await _channel!.ready;
      _isConnected = true;
      notifyListeners();

      // Listen for messages
      _channel!.stream.listen(
        (message) {
          _handleMessage(message);
        },
        onError: (error) {
          _isConnected = false;
          notifyListeners();
        },
        onDone: () {
          _isConnected = false;
          notifyListeners();
        },
      );
    } catch (e) {
      _isConnected = false;
      notifyListeners();
      rethrow;
    }
  }

  void disconnect() {
    _channel?.sink.close();
    _channel = null;
    _isConnected = false;
    _peers.clear();
    _chats.clear();
    _messages.clear();
    _selectedChat = null;
    notifyListeners();
  }

  void _handleMessage(dynamic message) {
    if (message is! String) return;

    try {
      final data = jsonDecode(message) as Map<String, dynamic>;
      final type = data['type'] as String?;
      final payload = data['payload'] as Map<String, dynamic>?;

      if (payload == null) return;

      switch (type) {
        case 'peers':
          final peersList = payload['peers'] as List?;
          if (peersList != null) {
            _peers = peersList
                .map((p) => Peer.fromJson(p as Map<String, dynamic>))
                .toList();
            notifyListeners();
          }
          break;

        case 'chats':
          final chatsList = payload['chats'] as List?;
          if (chatsList != null) {
            _chats = chatsList
                .map((c) => Chat.fromJson(c as Map<String, dynamic>))
                .toList();
            notifyListeners();
          }
          break;

        case 'messages':
          final chatId = payload['chat_id'] as String?;
          final messagesList = payload['messages'] as List?;
          if (chatId != null && messagesList != null) {
            _messages[chatId] = messagesList
                .map((m) => Message.fromJson(m as Map<String, dynamic>))
                .toList();
            notifyListeners();
          }
          break;

        case 'new_message':
          final msg = Message.fromJson(payload);
          final chatMessages = _messages[msg.chatId] ?? [];
          chatMessages.add(msg);
          _messages[msg.chatId] = chatMessages;
          
          // Update chat list
          _updateChatLastMessage(msg);
          notifyListeners();
          break;

        case 'group_created':
          // Refresh chats
          _sendRequest('get_chats', {});
          break;

        case 'error':
          debugPrint('Error: ${payload['message']}');
          break;
      }
    } catch (e) {
      debugPrint('Error handling message: $e');
    }
  }

  void _updateChatLastMessage(Message msg) {
    final chatIndex = _chats.indexWhere((c) => c.id == msg.chatId);
    if (chatIndex != -1) {
      final oldChat = _chats[chatIndex];
      _chats[chatIndex] = Chat(
        id: oldChat.id,
        name: oldChat.name,
        isGroup: oldChat.isGroup,
        participants: oldChat.participants,
        unreadCount: msg.senderId != _nodeId ? oldChat.unreadCount + 1 : oldChat.unreadCount,
        lastMessage: msg.content,
        lastTime: msg.timestamp,
      );
    }
  }

  void selectChat(Chat chat) {
    _selectedChat = chat;
    // Request messages for this chat
    _sendRequest('get_messages', {'chat_id': chat.id});
    notifyListeners();
  }

  void sendMessage(String content) {
    if (_selectedChat == null || content.isEmpty) return;

    String recipientId = 'ALL';
    if (!_selectedChat!.isGroup && _selectedChat!.id != 'ALL') {
      // Direct chat - extract recipient from chat ID
      final parts = _selectedChat!.id.split(':');
      if (parts.length == 3) {
        recipientId = parts[1] == _nodeId ? parts[2] : parts[1];
      }
    } else if (!_selectedChat!.isGroup) {
      recipientId = 'ALL';
    } else {
      recipientId = _selectedChat!.id;
    }

    _sendRequest('send_message', {
      'chat_id': _selectedChat!.id,
      'content': content,
      'recipient_id': recipientId,
    });
  }

  void createGroup(String name, List<String> participants) {
    _sendRequest('create_group', {
      'name': name,
      'participants': participants,
    });
  }

  void refreshChats() {
    _sendRequest('get_chats', {});
  }

  void refreshPeers() {
    _sendRequest('get_peers', {});
  }

  void _sendRequest(String type, Map<String, dynamic> payload) {
    if (!_isConnected || _channel == null) return;

    final message = jsonEncode({
      'type': type,
      'payload': payload,
    });
    _channel!.sink.add(message);
  }
}
