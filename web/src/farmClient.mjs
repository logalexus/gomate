// POSIX shell quoting keeps credentials and file names literal when copied.
export const shellQuote = value => "'" + String(value).replaceAll("'", "'\\''") + "'";
export function farmClientCommand({server, username, password, filename}) {
 return `python3 gomate_client.py --server ${shellQuote(server)} --user=${shellQuote(username)} --password=${shellQuote(password)} --name ${shellQuote(filename)} -- python3 ${shellQuote(filename)}`;
}
