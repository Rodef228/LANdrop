class Peer {
  final String id;
  final String name;
  final String ip;
  final int port;

  Peer({
    required this.id,
    required this.name,
    required this.ip,
    required this.port,
  });

  factory Peer.fromJson(Map<String, dynamic> json) {
    return Peer(
      id: json['id'] ?? '',
      name: json['name'] ?? '',
      ip: json['ip'] ?? '',
      port: json['port'] ?? 0,
    );
  }
}

class Chat {
  final String id;
  final String name;
  final bool isGroup;
  final String participants;
  final int unreadCount;
  final String? lastMessage;
  final int? lastTime;

  Chat({
    required this.id,
    required this.name,
    required this.isGroup,
    required this.participants,
    required this.unreadCount,
    this.lastMessage,
    this.lastTime,
  });

  factory Chat.fromJson(Map<String, dynamic> json) {
    return Chat(
      id: json['id'] ?? '',
      name: json['name'] ?? '',
      isGroup: json['is_group'] ?? false,
      participants: json['participants'] ?? '',
      unreadCount: json['unread_count'] ?? 0,
      lastMessage: json['last_message'],
      lastTime: json['last_time'],
    );
  }
}

class Message {
  final int? id;
  final String chatId;
  final String senderId;
  final String senderName;
  final String content;
  final int timestamp;
  final bool isRead;

  Message({
    this.id,
    required this.chatId,
    required this.senderId,
    required this.senderName,
    required this.content,
    required this.timestamp,
    required this.isRead,
  });

  factory Message.fromJson(Map<String, dynamic> json) {
    return Message(
      id: json['id'],
      chatId: json['chat_id'] ?? '',
      senderId: json['sender_id'] ?? '',
      senderName: json['sender_name'] ?? '',
      content: json['content'] ?? '',
      timestamp: json['timestamp'] ?? 0,
      isRead: json['is_read'] ?? false,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'chat_id': chatId,
      'sender_id': senderId,
      'sender_name': senderName,
      'content': content,
      'timestamp': timestamp,
      'is_read': isRead,
    };
  }
}
