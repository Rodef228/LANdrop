import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../services/mesh_service.dart';
import '../models/models.dart';
import '../widgets/chat_list.dart';
import '../widgets/message_list.dart';
import '../widgets/peer_list.dart';

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key});

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  final TextEditingController _messageController = TextEditingController();
  final TextEditingController _serverController = TextEditingController(text: 'localhost');
  final TextEditingController _portController = TextEditingController(text: '8765');

  bool _showSettings = true;

  @override
  void dispose() {
    _messageController.dispose();
    _serverController.dispose();
    _portController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Consumer<MeshService>(
      builder: (context, service, child) {
        return Scaffold(
          appBar: AppBar(
            title: const Text('Mesh-CU Messenger'),
            backgroundColor: Theme.of(context).colorScheme.inversePrimary,
            actions: [
              IconButton(
                icon: Icon(service.isConnected ? Icons.cloud_done : Icons.cloud_off),
                onPressed: () {
                  setState(() => _showSettings = !_showSettings);
                },
                tooltip: service.isConnected ? 'Connected' : 'Disconnected',
              ),
              if (service.isConnected)
                IconButton(
                  icon: const Icon(Icons.refresh),
                  onPressed: () {
                    service.refreshChats();
                    service.refreshPeers();
                  },
                  tooltip: 'Refresh',
                ),
            ],
          ),
          body: Column(
            children: [
              if (_showSettings) _buildConnectionPanel(service),
              Expanded(
                child: service.isConnected
                    ? Row(
                        children: [
                          SizedBox(
                            width: 280,
                            child: Column(
                              children: [
                                const Padding(
                                  padding: EdgeInsets.all(8.0),
                                  child: Text('Chats', style: TextStyle(fontWeight: FontWeight.bold)),
                                ),
                                Expanded(child: ChatListWidget()),
                                const Divider(height: 1),
                                SizedBox(
                                  height: 200,
                                  child: Column(
                                    children: [
                                      const Padding(
                                        padding: EdgeInsets.all(8.0),
                                        child: Text('Peers', style: TextStyle(fontWeight: FontWeight.bold)),
                                      ),
                                      Expanded(child: PeerListWidget()),
                                    ],
                                  ),
                                ),
                              ],
                            ),
                          ),
                          const VerticalDivider(width: 1),
                          Expanded(child: MessageListWidget(messageController: _messageController)),
                        ],
                      )
                    : const Center(
                        child: Column(
                          mainAxisAlignment: MainAxisAlignment.center,
                          children: [
                            Icon(Icons.cloud_off_outlined, size: 64, color: Colors.grey),
                            SizedBox(height: 16),
                            Text('Not connected', style: TextStyle(fontSize: 18, color: Colors.grey)),
                            SizedBox(height: 8),
                            Text('Configure connection settings above and click Connect'),
                          ],
                        ),
                      ),
              ),
            ],
          ),
        );
      },
    );
  }

  Widget _buildConnectionPanel(MeshService service) {
    return Container(
      padding: const EdgeInsets.all(16),
      color: Theme.of(context).colorScheme.surfaceContainerHighest,
      child: Row(
        children: [
          const Text('Server:'),
          const SizedBox(width: 8),
          SizedBox(
            width: 200,
            child: TextField(
              controller: _serverController,
              decoration: const InputDecoration(
                labelText: 'Host',
                contentPadding: EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                isDense: true,
              ),
            ),
          ),
          const SizedBox(width: 8),
          SizedBox(
            width: 100,
            child: TextField(
              controller: _portController,
              decoration: const InputDecoration(
                labelText: 'Port',
                contentPadding: EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                isDense: true,
              ),
            ),
          ),
          const SizedBox(width: 16),
          ElevatedButton.icon(
            icon: Icon(service.isConnected ? Icons.refresh : Icons.connect_without_contact),
            label: Text(service.isConnected ? 'Reconnect' : 'Connect'),
            onPressed: () {
              service.setServerAddress(_serverController.text, int.tryParse(_portController.text) ?? 8765);
              if (service.isConnected) {
                service.disconnect();
              }
              service.connect().catchError((e) {
                ScaffoldMessenger.of(context).showSnackBar(
                  SnackBar(content: Text('Connection failed: $e')),
                );
              });
            },
          ),
          if (service.isConnected) ...[
            const SizedBox(width: 8),
            OutlinedButton.icon(
              icon: const Icon(Icons.disconnect_outlined),
              label: const Text('Disconnect'),
              onPressed: () {
                service.disconnect();
              },
            ),
            const SizedBox(width: 16),
            Text('Node: ${service.nodeId.isEmpty ? "Loading..." : service.nodeId}'),
          ],
        ],
      ),
    );
  }
}
