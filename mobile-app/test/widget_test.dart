import 'package:flutter_test/flutter_test.dart';
import 'package:eve_assistant_mobile/main.dart';
void main() { testWidgets('app renders', (tester) async { await tester.pumpWidget(const EveAssistantApp()); expect(find.text('EVE 助手'), findsOneWidget); }); }
