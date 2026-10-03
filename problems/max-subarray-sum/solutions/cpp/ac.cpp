#include <algorithm>
#include <iostream>

int main() {
    std::ios::sync_with_stdio(false);
    std::cin.tie(nullptr);
    int n;
    std::cin >> n;
    long long best, cur;
    std::cin >> best;
    cur = best;
    for (int i = 1; i < n; i++) {
        long long x;
        std::cin >> x;
        cur = std::max(x, cur + x);
        best = std::max(best, cur);
    }
    std::cout << best << "\n";
}
