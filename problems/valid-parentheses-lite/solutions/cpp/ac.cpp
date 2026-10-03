#include <iostream>
#include <string>
#include <vector>

int main() {
    std::string s;
    std::cin >> s;
    std::vector<char> st;
    bool ok = true;
    for (char ch : s) {
        if (ch == '(' || ch == '[') {
            st.push_back(ch);
        } else {
            char want = ch == ')' ? '(' : '[';
            if (st.empty() || st.back() != want) {
                ok = false;
                break;
            }
            st.pop_back();
        }
    }
    std::cout << (ok && st.empty() ? "YES" : "NO") << "\n";
}
