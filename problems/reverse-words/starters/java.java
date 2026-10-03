import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;

public class Main {
    static String[] solve(String[] words) {
        // return the words in reverse order
        return words;
    }

    public static void main(String[] args) throws IOException {
        BufferedReader in = new BufferedReader(new InputStreamReader(System.in));
        in.readLine();
        String[] words = in.readLine().trim().split("\\s+");
        System.out.println(String.join(" ", solve(words)));
    }
}
