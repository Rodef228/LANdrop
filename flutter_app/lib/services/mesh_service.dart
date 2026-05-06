import 'dart:convert';
import 'package:http/http.dart' as http;

// Классы Message и Peer определены здесь же, чтобы не было конфликтов импортов
class Message {
  final String id;
  final String chatId;
  final String senderId;
  final String content;
  final DateTime timestamp;

  Message({
    required this.id,
    required this.chatId,
    required this.senderId,
    required this.content,
    required this.timestamp,
  });

  factory Message.fromJson(Map<String, dynamic> json) {
    return Message(
      id: json['id'] ?? '',
      chatId: json['chat_id'] ?? '',
      senderId: json['sender_id'] ?? '',
      content: json['content'] ?? '',
      timestamp: DateTime.tryParse(json['timestamp'] ?? '') ?? DateTime.now(),
    );
  }
}

class Peer {
  final String id;
  final String address;
  final bool isActive;

  Peer({required this.id, required this.address, required this.isActive});

  factory Peer.fromJson(Map<String, dynamic> json) {
    return Peer(
      id: json['id'] ?? '',
      address: json['address'] ?? '',
      isActive: json['is_active'] ?? false,
    );
  }
}

class MeshService {
  static const String baseUrl = 'http://localhost:8765/api';

  Future<List<Map<String, dynamic>>> getChats() async {
    try {
      final response = await http.get(Uri.parse('$baseUrl/chats'));
      if (response.statusCode == 200) {
        return List<Map<String, dynamic>>.from(json.decode(response.body));
      } else {
        throw Exception('Failed to load chats');
      }
    } catch (e) {
      print('Error fetching chats: $e');
      return [];
    }
  }

  Future<List<Message>> getMessages(String chatId) async {
    try {
      final response = await http.get(Uri.parse('$baseUrl/messages/$chatId'));
      if (response.statusCode == 200) {
        final List<dynamic> jsonList = json.decode(response.body);
        return jsonList.map((json) => Message.fromJson(json)).toList();
      } else {
        return [];
      }
    } catch (e) {
      print('Error fetching messages: $e');
      return [];
    }
  }

  Future<void> sendMessage(
      String chatId, String content, String senderId) async {
    try {
      final response = await http.post(
        Uri.parse('$baseUrl/messages'),
        headers: {'Content-Type': 'application/json'},
        body: json.encode({
          'chat_id': chatId,
          'sender_id': senderId,
          'content': content,
        }),
      );
      if (response.statusCode != 200 && response.statusCode != 201) {
        throw Exception('Failed to send message');
      }
    } catch (e) {
      print('Error sending message: $e');
    }
  }

  Future<List<Peer>> getPeers() async {
    try {
      final response = await http.get(Uri.parse('$baseUrl/peers'));
      if (response.statusCode == 200) {
        final List<dynamic> jsonList = json.decode(response.body);
        return jsonList.map((json) => Peer.fromJson(json)).toList();
      } else {
        return [];
      }
    } catch (e) {
      print('Error fetching peers: $e');
      return [];
    }
  }
}
