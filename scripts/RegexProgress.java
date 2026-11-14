// Bounded reproduction of PatternMatcher.matchRegex's progress problem.
// This diagnoses the algorithm, not the cause of any particular historical hang.
import java.util.regex.Pattern;
class RegexProgress {
    public static void main(String[] args) {
        var matcher = Pattern.compile("a*").matcher("abc");
        int startPos = 0;
        for (int i = 0; i < 5 && matcher.find(startPos); i++) {
            int next = matcher.end();
            if (next == startPos) {
                System.out.println("REPRODUCED: zero-length match does not advance startPos=" + startPos);
                return;
            }
            startPos = next;
        }
        throw new AssertionError("expected non-advancing match");
    }
}
