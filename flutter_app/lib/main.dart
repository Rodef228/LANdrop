import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import 'dart:convert';

void main() {
  runApp(const MeshChatApp());
}

class MeshChatApp extends StatelessWidget {
  const MeshChatApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Mesh Chat',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: Colors.deepPurple),
        useMaterial3: true,
        brightness: Brightness.dark, // Темная тема для хакерского вайба
      ),
      home: const ChatScreen(),
    );
  }
}

class ChatScreen extends StatefulWidget {
  const ChatScreen({super.key});

  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> {
  // ЗАМЕНИ НА АДРЕС ТВОЕГО GO СЕРВЕРА
  final String baseUrl = 'http://localhost:8080';

  List<dynamic> messages = [];
  List<dynamic> peers = [];
  final TextEditingController _msgController = TextEditingController();
  final TextEditingController _chatIdController = TextEditingController(
    text = 'global',
  );
  bool isLoading = false;

  @override
  void initState() {
    super.initState();
    _fetchData();
    // Обновляем данные каждые 3 секунды
    Future.periodic(const Duration(seconds: 3), (_) => _fetchData());
  }

  // Получение сообщений и пиров
  Future<void> _fetchData() async {
    setState(() => isLoading = true);
    try {
      // Пример эндпоинта для получения сообщений.
      // Подстрой путь под свой Go бэкенд (например /api/messages)
      final msgRes = await http.get(
        Uri.parse('$baseUrl/api/messages?chat_id=${_chatIdController.text}'),
      );

      // Пример эндпоинта для получения списка пиров
      final peerRes = await http.get(Uri.parse('$baseUrl/api/peers'));

      if (msgRes.statusCode == 200) {
        setState(() {
          messages = json.decode(msgRes.body);
        });
      }

      if (peerRes.statusCode == 200) {
        setState(() {
          peers = json.decode(peerRes.body);
        });
      }
    } catch (e) {
      print("Ошибка подключения: $e");
    } finally {
      setState(() => isLoading = false);
    }
  }

  // Отправка сообщения
  Future<void> _sendMessage() async {
    if (_msgController.text.isEmpty) return;

    try {
      final response = await http.post(
        Uri.parse('$baseUrl/api/send'), // Подстрой путь под свой бэкенд
        headers: {'Content-Type': 'application/json'},
        body: json.encode({
          'chat_id': _chatIdController.text,
          'text': _msgController.text,
          // 'sender': 'user_web', // Если нужно указывать отправителя
        }),
      );

      if (response.statusCode == 200 || response.statusCode == 201) {
        _msgController.clear();
        _fetchData(); // Сразу обновим список
      } else {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Ошибка отправки: ${response.statusCode}')),
        );
      }
    } catch (e) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('Нет связи с сервером: $e')));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Mesh Node Interface'),
        backgroundColor: Theme.of(context).colorScheme.inversePrimary,
        actions: [
          IconButton(icon: const Icon(Icons.refresh), onPressed: _fetchData),
        ],
      ),
      body: Column(
        children: [
          // Панель управления (выбор чата)
          Padding(
            padding: const EdgeInsets.all(8.0),
            child: Row(
              children: [
                const Text('Чат ID: '),
                Expanded(
                  child: TextField(
                    controller: _chatIdController,
                    decoration: const InputDecoration(
                      hintText: 'global, user123...',
                      border: OutlineInputBorder(),
                      isDense: true,
                    ),
                  ),
                ),
              ],
            ),
          ),

          // Список пиров (сверху)
          if (peers.isNotEmpty)
            Container(
              height: 50,
              padding: const EdgeInsets.symmetric(horizontal: 8),
              child: ListView.builder(
                scrollDirection: Axis.horizontal,
                itemCount: peers.length,
                itemBuilder: (ctx, i) => Chip(
                  label: Text(peers[i]['id'] ?? 'Peer $i'),
                  avatar: const CircleAvatar(
                    radius: 10,
                    backgroundColor: Colors.green,
                  ),
                  margin: const EdgeInsets.only(right: 5),
                ),
              ),
            ),

          // Список сообщений
          Expanded(
            child: isLoading && messages.isEmpty
                ? const Center(child: CircularProgressIndicator())
                : ListView.builder(
                    itemCount: messages.length,
                    itemBuilder: (ctx, i) {
                      final msg = messages[i];
                      // Адаптируй поля под свой JSON (text, sender, timestamp)
                      final text = msg['text'] ?? msg['content'] ?? 'No text';
                      final sender = msg['sender'] ?? msg['from'] ?? 'Unknown';

                      return ListTile(
                        leading: CircleAvatar(
                          child: Text(sender[0].toUpperCase()),
                        ),
                        title: Text(text),
                        subtitle: Text('От: $sender'),
                        isThreeLine: true,
                      );
                    },
                  ),
          ),

          // Поле ввода
          Padding(
            padding: const EdgeInsets.all(8.0),
            child: Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _msgController,
                    decoration: const InputDecoration(
                      hintText: 'Введите сообщение...',
                      border: OutlineInputBorder(),
                    ),
                    onSubmitted: (_) => _sendMessage(),
                  ),
                ),
                const SizedBox(width: 8),
                IconButton.filled(
                  icon: const Icon(Icons.send),
                  onPressed: _sendMessage,
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
