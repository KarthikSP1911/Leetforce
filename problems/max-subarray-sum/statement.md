Given an array of `n` integers, find the largest sum of a non-empty contiguous block of the array.

## Input

- The first line contains `n` (`1 <= n <= 100000`).
- The second line contains `n` integers `a[i]` (`-10^9 <= a[i] <= 10^9`).

## Output

A single integer: the largest sum over all non-empty contiguous subarrays.

## Example

Input:

```
9
-2 1 -3 4 -1 2 1 -5 4
```

Output:

```
6
```

The best block is `4 -1 2 1`.

## Notes

- The block must contain at least one element, so if every number is negative the answer is the largest single element.
- Sums can exceed 32 bits (up to `10^14`); use a 64-bit integer.
- An `O(n)` solution exists.
