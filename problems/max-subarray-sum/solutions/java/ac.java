import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.util.StringTokenizer;

public class Main {
    public static void main(String[] args) throws IOException {
        BufferedReader in = new BufferedReader(new InputStreamReader(System.in));
        int n = Integer.parseInt(in.readLine().trim());
        StringTokenizer t = new StringTokenizer(in.readLine());
        long best = Long.parseLong(t.nextToken());
        long cur = best;
        for (int i = 1; i < n; i++) {
            long x = Long.parseLong(t.nextToken());
            cur = Math.max(x, cur + x);
            best = Math.max(best, cur);
        }
        System.out.println(best);
    }
}
