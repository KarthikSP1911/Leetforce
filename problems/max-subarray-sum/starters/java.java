import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.util.StringTokenizer;

public class Main {
    static long solve(long[] a) {
        // return the largest sum of a non-empty contiguous subarray
        return 0;
    }

    public static void main(String[] args) throws IOException {
        BufferedReader in = new BufferedReader(new InputStreamReader(System.in));
        int n = Integer.parseInt(in.readLine().trim());
        StringTokenizer t = new StringTokenizer(in.readLine());
        long[] a = new long[n];
        for (int i = 0; i < n; i++) a[i] = Long.parseLong(t.nextToken());
        System.out.println(solve(a));
    }
}
