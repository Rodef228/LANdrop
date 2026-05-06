import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../services/mesh_service.dart';

class PeerListWidget extends StatelessWidget {
  const PeerListWidget({super.key});

  @override
  Widget build(BuildContext context) {
    return Consumer<MeshService>(
      builder: (context, service, child) {
        final peers = service.peers;

        if (peers.isEmpty) {
          return const Center(
            child: Text('No peers', style: TextStyle(color: Colors.grey, fontSize: 12)),
          );
        }

        return ListView.builder(
          itemCount: peers.length,
          itemBuilder: (context, index) {
            final peer = peers[index];

            return ListTile(
              dense: true,
              leading: const CircleAvatar(
                radius: 16,
                backgroundColor: Colors.green,
                child: Icon(Icons.person, size: 16, color: Colors.white),
              ),
              title: Text(
                peer.name,
                style: const TextStyle(fontSize: 13),
              ),
              subtitle: Text(
                '${peer.ip}:${peer.port}',
                style: TextStyle(color: Colors.grey.shade600, fontSize: 11),
              ),
              trailing: Container(
                width: 8,
                height: 8,
                decoration: const BoxDecoration(
                  color: Colors.green,
                  shape: BoxShape.circle,
                ),
              ),
            );
          },
        );
      },
    );
  }
}
