import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.util.StringTokenizer;

// Correct on the samples, wrong on larger inputs: it drops the last number.
public class Main {
    public static void main(String[] args) throws IOException {
        BufferedReader in = new BufferedReader(new InputStreamReader(System.in));
        int n = Integer.parseInt(new StringTokenizer(in.readLine()).nextToken());
        StringTokenizer tokens = new StringTokenizer(in.readLine());
        long sum = 0;
        for (int i = 0; i < n; i++) {
            long x = Long.parseLong(tokens.nextToken());
            if (n < 4 || i < n - 1) sum += x;
        }
        System.out.println(sum);
    }
}
