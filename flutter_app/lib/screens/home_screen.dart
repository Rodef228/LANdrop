import 'package:flutter/material.dart';
import '../services/mesh_service.dart';
import 'chat_screen.dart';
import 'dart:async';

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key});

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  final MeshService _service = MeshService();
  List<Map<String, dynamic>> _chats = [];
  List<Peer> _peers = [];
  bool _isLoading = true;

  @override
  void initState() {
    super.initState();
    _refreshData();
    // Исправлено: теперь используем Timer.periodic, а не Future.periodic
    Future.delayed(Duration.zero, () {
      Timer.periodic(const Duration(seconds: 5), (timer) {
        if (!mounted) {
          timer.cancel();
          return;
        }
        _refreshData();
      });
    });
  }

  Future<void> _refreshData() async {
    if (!mounted) return;
    setState(() => _isLoading = true);

    await Future.wait([
      _loadChats(),
      _loadPeers(),
    ]);

    if (mounted) setState(() => _isLoading = false);
  }

  Future<void> _loadChats() async {
    final chats = await _service.getChats();
    if (mounted) setState(() => _chats = chats);
  }

  Future<void> _loadPeers() async {
    final peers = await _service.getPeers();
    if (mounted) setState(() => _peers = peers);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Mesh Network'),
        backgroundColor: Theme.of(context).colorScheme.inversePrimary,
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh),
            onPressed: _refreshData,
            tooltip: 'Обновить',
          ),
          Padding(
            padding: const EdgeInsets.only(right: 16.0),
            child: Center(
              child: Text(
                'Пиров: ${_peers.length}',
                style: TextStyle(
                    fontSize: 14,
                    color: Theme.of(context).colorScheme.onSurfaceVariant),
              ),
            ),
          ),
        ],
      ),
      body: _isLoading
          ? const Center(child: CircularProgressIndicator())
          : _chats.isEmpty
              ? const Center(child: Text('Нет активных чатов'))
              : ListView.builder(
                  itemCount: _chats.length,
                  itemBuilder: (context, index) {
                    final chat = _chats[index];
                    return ListTile(
                      leading: const CircleAvatar(
                        child: Icon(Icons.group),
                      ),
                      title: Text(chat['name'] ?? 'Чат ${chat['id']}'),
                      subtitle: Text('ID: ${chat['id']}'),
                      trailing: const Icon(Icons.chevron_right),
                      onTap: () {
                        Navigator.push(
                          context,
                          MaterialPageRoute(
                            builder: (context) => ChatScreen(
                              chatId: chat['id'],
                              chatName: chat['name'] ?? 'Чат',
                            ),
                          ),
                        );
                      },
                    );
                  },
                ),
      floatingActionButton: FloatingActionButton(
        onPressed: () {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(content: Text('Создание чата пока не реализовано')),
          );
        },
        child: const Icon(Icons.add),
      ),
    );
  }
}
