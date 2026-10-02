int main() {
    volatile unsigned long counter = 0;
    for (;;) counter = counter + 1;
}
