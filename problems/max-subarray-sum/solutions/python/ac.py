import sys

data = sys.stdin.read().split()
n = int(data[0])
a = list(map(int, data[1 : n + 1]))
best = cur = a[0]
for x in a[1:]:
    cur = max(x, cur + x)
    best = max(best, cur)
print(best)
