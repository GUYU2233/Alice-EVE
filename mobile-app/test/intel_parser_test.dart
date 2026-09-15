import 'package:flutter_test/flutter_test.dart';
import 'package:eve_assistant_mobile/intel/intel_parser.dart';
void main(){test('parses intel fields',(){final t=DateTime.utc(2025,1,1);final r=IntelParser().parse('Hostile Rifter spotted in Jita',now:t);expect(r.map((x)=>x.value),containsAll(['Jita','Rifter','hostile']));expect(r.first.expiresAt,t.add(const Duration(minutes:30)));expect(r.first.source,'clipboard');});}
