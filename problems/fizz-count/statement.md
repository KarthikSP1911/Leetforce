Given a positive integer `n`, count the integers `k` with `1 <= k <= n` that are divisible by exactly one of `3` and `5` (divisible by `3` or by `5`, but not by both).

## Input

A single integer `n` (`1 <= n <= 10^9`).

## Output

A single integer: the count.

## Example

Input:

```
10
```

Output:

```
5
```

The numbers are 3, 5, 6, 9 and 10. The number 15 would be excluded because it is divisible by both.

## Constraints

- `1 <= n <= 10^9`, so counting one number at a time is too slow in a slow language; think about how many multiples of each number fit below `n`.
