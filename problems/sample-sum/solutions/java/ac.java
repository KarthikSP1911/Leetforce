import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.util.StringTokenizer;

public class Main {
    public static void main(String[] args) throws IOException {
        BufferedReader in = new BufferedReader(new InputStreamReader(System.in));
        StringTokenizer tokens = new StringTokenizer(in.readLine());
        int n = Integer.parseInt(tokens.nextToken());
        long sum = 0;
        tokens = new StringTokenizer(in.readLine());
        for (int i = 0; i < n; i++) {
            sum += Long.parseLong(tokens.nextToken());
        }
        System.out.println(sum);
    }
}
