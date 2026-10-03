import sys

lines = sys.stdin.read().split("\n")
print(" ".join(reversed(lines[1].split())))
