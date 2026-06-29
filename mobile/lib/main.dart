import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:hive_flutter/hive_flutter.dart';
import 'src/app.dart';
import 'src/providers/auth_provider.dart';
import 'src/services/api_service.dart';
import 'src/services/websocket_service.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();

  // Initialize Hive for local storage
  await Hive.initFlutter();
  await Hive.openBox('settings');
  await Hive.openBox('cache');

  runApp(
    const ProviderScope(
      child: IraqSecureChatApp(),
    ),
  );
}

class IraqSecureChatApp extends ConsumerWidget {
  const IraqSecureChatApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final themeMode = ref.watch(themeProvider);
    final locale = ref.watch(localeProvider);

    return MaterialApp(
      title: 'IraqSecureChat',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF1A73E8),
          brightness: Brightness.light,
        ),
        fontFamily: 'Tajawal',
        useMaterial3: true,
      ),
      darkTheme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF1A73E8),
          brightness: Brightness.dark,
        ),
        fontFamily: 'Tajawal',
        useMaterial3: true,
      ),
      themeMode: themeMode,
      locale: locale,
      supportedLocales: const [
        Locale('ar', 'IQ'),
        Locale('en', 'US'),
        Locale('ku', 'IQ'), // Kurdish
        Locale('ckb', 'IQ'), // Sorani Kurdish
      ],
      localizationsDelegates: const [
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      localeResolutionCallback: (locale, supportedLocales) {
        if (locale != null) {
          for (final supported in supportedLocales) {
            if (supported.languageCode == locale.languageCode) {
              return supported;
            }
          }
        }
        return const Locale('ar', 'IQ');
      },
      home: const AuthGate(),
    );
  }
}

class AuthGate extends ConsumerWidget {
  const AuthGate({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final authState = ref.watch(authProvider);

    return authState.when(
      loading: () => const SplashScreen(),
      authenticated: (_) => const MainScreen(),
      unauthenticated: () => const LoginScreen(),
    );
  }
}

class SplashScreen extends StatelessWidget {
  const SplashScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Theme.of(context).colorScheme.primary,
      body: Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Container(
              width: 80,
              height: 80,
              decoration: BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.circular(20),
              ),
              child: const Icon(
                Icons.shield_outlined,
                size: 40,
                color: Color(0xFF1A73E8),
              ),
            ),
            const SizedBox(height: 24),
            const Text(
              'IraqSecureChat',
              style: TextStyle(
                color: Colors.white,
                fontSize: 28,
                fontWeight: FontWeight.bold,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              'المراسلة الآمنة للجهات الحكومية',
              style: TextStyle(
                color: Colors.white.withValues(alpha: 0.8),
                fontSize: 14,
              ),
            ),
            const SizedBox(height: 48),
            const CircularProgressIndicator(color: Colors.white),
          ],
        ),
      ),
    );
  }
}

class MainScreen extends StatelessWidget {
  const MainScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return const Scaffold(
      body: Center(child: Text('Main Screen - Chats')),
    );
  }
}

class LoginScreen extends StatelessWidget {
  const LoginScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Container(
                width: 80,
                height: 80,
                decoration: BoxDecoration(
                  color: Theme.of(context).colorScheme.primary,
                  borderRadius: BorderRadius.circular(20),
                ),
                child: const Icon(Icons.shield_outlined, size: 40, color: Colors.white),
              ),
              const SizedBox(height: 32),
              Text(
                'IraqSecureChat',
                style: Theme.of(context).textTheme.headlineMedium?.copyWith(
                  fontWeight: FontWeight.bold,
                ),
              ),
              const SizedBox(height: 8),
              Text(
                'تواصل آمن للجهات الحكومية العراقية',
                style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                  color: Colors.grey,
                ),
              ),
              const SizedBox(height: 48),
              TextField(
                decoration: InputDecoration(
                  labelText: 'Phone Number',
                  hintText: '+9647XXXXXXXX',
                  prefixText: '+964 ',
                  border: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(12),
                  ),
                ),
                keyboardType: TextInputType.phone,
                textDirection: TextDirection.ltr,
              ),
              const SizedBox(height: 16),
              SizedBox(
                width: double.infinity,
                height: 48,
                child: ElevatedButton(
                  onPressed: () {},
                  style: ElevatedButton.styleFrom(
                    backgroundColor: Theme.of(context).colorScheme.primary,
                    foregroundColor: Colors.white,
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(12),
                    ),
                  ),
                  child: const Text('Send Code / إرسال الرمز'),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// Providers
final themeProvider = StateProvider<ThemeMode>((ref) => ThemeMode.light);
final localeProvider = StateProvider<Locale>((ref) => const Locale('ar', 'IQ'));

final authProvider = FutureProvider<AuthState>((ref) async {
  // Dummy auth state for now
  await Future.delayed(const Duration(seconds: 2));
  return AuthState.unauthenticated();
});

enum AuthState { loading, authenticated, unauthenticated }
