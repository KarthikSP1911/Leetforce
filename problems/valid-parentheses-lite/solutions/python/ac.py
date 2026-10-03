s = input().strip()
stack = []
pairs = {")": "(", "]": "["}
ok = True
for ch in s:
    if ch in "([":
        stack.append(ch)
    elif not stack or stack.pop() != pairs[ch]:
        ok = False
        break
print("YES" if ok and not stack else "NO")
