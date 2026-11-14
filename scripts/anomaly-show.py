import json, sys
rows = json.load(sys.stdin)
print(len(rows), 'rows')
for a in rows[:int(sys.argv[1]) if len(sys.argv) > 1 else 40]:
    print('%6.1f %-6s #%-5d %s' % (a['score'], a['kind'], a['streamId'], a['summary'][:80]))
    for r in a['reasons'][:4]:
        print('        +%.1f %-8s %-18s %s' % (r['bits'], r['detector'], r.get('field', ''), r['text'][:120]))
