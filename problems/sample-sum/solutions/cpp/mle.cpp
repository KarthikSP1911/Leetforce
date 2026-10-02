#include <cstdio>
#include <cstdlib>

int main() {
    const size_t size = 600UL << 20;
    volatile char* block = static_cast<volatile char*>(std::malloc(size));
    for (size_t i = 0; i < size; i++) block[i] = 1;
    std::printf("%d\n", block[size / 2]);
}
