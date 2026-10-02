import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

public class Main {
    public static void main(String[] args) {
        List<byte[]> blocks = new ArrayList<>();
        for (int i = 0; i < 40; i++) {
            byte[] block = new byte[16 << 20];
            Arrays.fill(block, (byte) 1);
            blocks.add(block);
        }
        System.out.println(blocks.size());
    }
}
