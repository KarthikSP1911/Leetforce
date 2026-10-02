import sys

data = sys.stdin.read().split()
n = int(data[0])
print(sum(int(x) for x in data[1 : n + 1]))
