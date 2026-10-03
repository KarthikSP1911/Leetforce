import java.util.Scanner;

public class Main {
    public static void main(String[] args) {
        long n = new Scanner(System.in).nextLong();
        System.out.println(n / 3 + n / 5 - 2 * (n / 15));
    }
}
