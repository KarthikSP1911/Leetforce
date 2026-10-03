#include <iostream>
#include <string>
#include <vector>

std::vector<std::string> solve(std::vector<std::string> words) {
    // return the words in reverse order
    return words;
}

int main() {
    int n;
    std::cin >> n;
    std::vector<std::string> words(n);
    for (auto& s : words) std::cin >> s;
    auto out = solve(words);
    for (size_t i = 0; i < out.size(); i++) {
        std::cout << out[i] << (i + 1 < out.size() ? " " : "\n");
    }
}
