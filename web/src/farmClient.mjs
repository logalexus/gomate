// POSIX shell quoting keeps credentials and file names literal when copied.
export const shellQuote = value => "'" + String(value).replaceAll("'", "'\\''") + "'";
export function farmClientCommand({server, username, password, filename}) {
 return `GOMATE_USER=${shellQuote(username)} GOMATE_PASSWORD=${shellQuote(password)} python3 gomate_client.py --server ${shellQuote(server)} --name ${shellQuote(filename)} -- python3 ${shellQuote(filename)}`;
}
