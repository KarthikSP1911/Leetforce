#include <iostream>
#include <string>
#include <vector>

int main() {
    int n;
    std::cin >> n;
    std::vector<std::string> w(n);
    for (auto& s : w) std::cin >> s;
    for (int i = n - 1; i >= 0; i--) {
        std::cout << w[i] << (i ? " " : "\n");
    }
}
