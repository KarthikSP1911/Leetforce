#include <iostream>
#include <vector>

long long solve(const std::vector<long long>& a) {
    // return the largest sum of a non-empty contiguous subarray
    return 0;
}

int main() {
    std::ios::sync_with_stdio(false);
    std::cin.tie(nullptr);
    int n;
    std::cin >> n;
    std::vector<long long> a(n);
    for (auto& x : a) std::cin >> x;
    std::cout << solve(a) << "\n";
}
