import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../services/mesh_service.dart';
import '../models/models.dart';

class ChatListWidget extends StatelessWidget {
  const ChatListWidget({super.key});

  @override
  Widget build(BuildContext context) {
    return Consumer<MeshService>(
      builder: (context, service, child) {
        final chats = service.chats;

        if (chats.isEmpty) {
          return const Center(
            child: Text('No chats', style: TextStyle(color: Colors.grey)),
          );
        }

        // Add "All" chat at the top
        final allChat = Chat(
          id: 'ALL',
          name: 'Global Chat',
          isGroup: false,
          participants: '',
          unreadCount: 0,
        );
        final allChats = [allChat, ...chats];

        return ListView.builder(
          itemCount: allChats.length,
          itemBuilder: (context, index) {
            final chat = allChats[index];
            final isSelected = service.selectedChat?.id == chat.id;

            return ListTile(
              selected: isSelected,
              leading: Icon(
                chat.isGroup ? Icons.group : Icons.person,
                color: isSelected 
                  ? Theme.of(context).colorScheme.primary 
                  : null,
              ),
              title: Text(
                chat.name,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
              subtitle: chat.lastMessage != null
                  ? Text(
                      chat.lastMessage!,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        fontWeight: chat.unreadCount > 0 ? FontWeight.bold : FontWeight.normal,
                        color: chat.unreadCount > 0 
                          ? Theme.of(context).colorScheme.primary 
                          : Colors.grey,
                      ),
                    )
                  : null,
              trailing: chat.unreadCount > 0
                  ? Chip(
                      label: Text('${chat.unreadCount}', style: const TextStyle(fontSize: 12, color: Colors.white)),
                      padding: EdgeInsets.zero,
                      materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
                      visualDensity: VisualDensity.compact,
                      backgroundColor: Theme.of(context).colorScheme.primary,
                    )
                  : null,
              onTap: () => service.selectChat(chat),
            );
          },
        );
      },
    );
  }
}
