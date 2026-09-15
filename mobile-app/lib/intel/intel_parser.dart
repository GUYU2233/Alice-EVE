class IntelFinding {
  final String kind, value, stance, source, summary;
  final DateTime observedAt, expiresAt;
  final double confidence;
  const IntelFinding({required this.kind, required this.value, required this.stance, required this.source, required this.summary, required this.observedAt, required this.expiresAt, required this.confidence});
  Map<String,dynamic> toJson()=>{'kind':kind,'value':value,'stance':stance,'source':source,'observedAt':observedAt.toUtc().toIso8601String(),'expiresAt':expiresAt.toUtc().toIso8601String(),'confidence':confidence,'summary':summary};
}

class IntelParser {
  static const systems = ['Jita','Amarr','Dodixie','Rens','Hek','Tama','Niarja','Uedama','Perigen Falls','Aldranette'];
  static const ships = ['Rifter','Merlin','Drake','Caracal','Raven','Megathron','Catalyst','Venture','Interceptor'];
  static const hostile = ['敌对','红名','hostile','neutral?','war target','neut'];
  static const friendly = ['友军','蓝名','friendly','allied','蓝'];

  List<IntelFinding> parse(String text, {String source='clipboard', DateTime? now, Duration ttl=const Duration(minutes: 30)}) {
    final observed = now ?? DateTime.now(); final out=<IntelFinding>[];
    for (final s in systems) { if (RegExp('\\b${RegExp.escape(s)}\\b', caseSensitive:false).hasMatch(text)) out.add(_f('system',s,text,source,observed,ttl,0.95)); }
    for (final s in ships) { if (RegExp('\\b${RegExp.escape(s)}\\b', caseSensitive:false).hasMatch(text)) out.add(_f('ship',s,text,source,observed,ttl,0.88)); }
    var stance='unknown';
    if (hostile.any((x)=>text.toLowerCase().contains(x.toLowerCase()))) {
      stance='hostile';
    } else if (friendly.any((x)=>text.toLowerCase().contains(x.toLowerCase()))) {
      stance='friendly';
    }
    if (stance!='unknown') out.add(_f('stance',stance,text,source,observed,ttl,0.82));
    return out;
  }
  IntelFinding _f(String kind,String value,String text,String source,DateTime at,Duration ttl,double confidence)=>IntelFinding(kind:kind,value:value,stance:kind=='stance'?value:'unknown',source:source,summary:'$value detected${kind=='stance'?' ($value)':''}',observedAt:at,expiresAt:at.add(ttl),confidence:confidence);
}
