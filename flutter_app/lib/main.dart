import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'services/mesh_service.dart';
import 'screens/home_screen.dart';

void main() {
  runApp(const MeshCUApp());
}

class MeshCUApp extends StatelessWidget {
  const MeshCUApp({super.key});

  @override
  Widget build(BuildContext context) {
    return ChangeNotifierProvider(
      create: (_) => MeshService(),
      child: MaterialApp(
        title: 'Mesh-CU Messenger',
        debugShowCheckedModeBanner: false,
        theme: ThemeData(
          colorScheme: ColorScheme.fromSeed(seedColor: Colors.blue),
          useMaterial3: true,
        ),
        home: const HomeScreen(),
      ),
    );
  }
}
