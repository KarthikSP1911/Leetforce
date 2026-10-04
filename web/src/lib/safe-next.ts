// Where to go after sign-in. The ?next= value comes from the URL, so only a
// same-site path is accepted; anything else falls back to the problem list.
//
// Browsers treat a backslash like a slash, so "/\evil.example" would leave the
// site just like "//evil.example"; control characters are refused for the same
// reason (they are stripped before the URL is parsed).
export function safeNext(next: string | null): string {
  if (
    next &&
    next.startsWith("/") &&
    !next.startsWith("//") &&
    !/[\\\u0000-\u001f\u007f]/.test(next)
  ) {
    return next;
  }
  return "/problems";
}
