A string made only of the characters `(`, `)`, `[` and `]` is *balanced* if every opening bracket is closed by a bracket of the same kind, in the correct order, and every closing bracket closes something.

Print `YES` if the given string is balanced and `NO` otherwise.

## Input

A single line holding a string of length `1` to `100000` made of `(`, `)`, `[` and `]`.

## Output

`YES` or `NO`.

## Examples

Input:

```
()[]
```

Output:

```
YES
```

Input:

```
([)]
```

Output:

```
NO
```

Here the `[` is still open when the `)` tries to close the `(`, so the nesting is wrong.

## Notes

A stack of the currently open brackets is enough: push on an opening bracket, and on a closing bracket check that the top matches. At the end the stack must be empty.
