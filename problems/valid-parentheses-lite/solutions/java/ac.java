import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;

public class Main {
    public static void main(String[] args) throws IOException {
        String s = new BufferedReader(new InputStreamReader(System.in)).readLine().trim();
        char[] st = new char[s.length()];
        int top = 0;
        boolean ok = true;
        for (int i = 0; i < s.length() && ok; i++) {
            char ch = s.charAt(i);
            if (ch == '(' || ch == '[') {
                st[top++] = ch;
            } else {
                char want = ch == ')' ? '(' : '[';
                if (top == 0 || st[top - 1] != want) ok = false;
                else top--;
            }
        }
        System.out.println(ok && top == 0 ? "YES" : "NO");
    }
}
