import {test} from 'node:test';
import assert from 'node:assert/strict';
import {renderChatMarkdown as render, insideCodeFence} from '../../web/src/chatMarkdown.mjs';

test('formats prose, lists, quotations, links and code without altering code whitespace', () => {
 const html=render('**bold** *italic* ~~gone~~ `inline`\n\n- one\n- two\n\n> quote\n\n[docs](https://example.com)\n\n```go\n  if a < b {\n\tprint("<script>")\n  }\n```');
 for(const fragment of ['<strong>bold</strong>','<em>italic</em>','<s>gone</s>','<code>inline</code>','<ul>','<blockquote>','target="_blank"','rel="noopener noreferrer"','chat-copy-code','  if a &lt; b {\n\tprint(&quot;&lt;script&gt;&quot;)'])assert(html.includes(fragment),fragment);
 assert(render('one\ntwo').includes('<br>'));
});

test('HTML, fence metadata and dangerous links cannot introduce active content', () => {
 const cases=['<img src=x onerror=alert(1)>','<svg/onload=alert(1)>','<script>alert(1)</script>','[x](javascript:alert%281%29)','[x](JaVaScRiPt:alert%281%29)','[x](jav&#x61;script:alert%281%29)','[x](data:text/html;base64,PHNjcmlwdD4=)','[x](file:///etc/passwd)','[x](//example.com)','```"><img/src=x/onerror=alert(1)>\n<script>test</script>\n```'];
 assert(!/<img\b/i.test(render('![track](https://example.com/pixel)')));
 for(const value of cases){const html=render(value);assert(!/<(?:script|img|svg|iframe)\b/i.test(html),html);assert(!/href=/i.test(html),html)}
});

test('unfinished fences and long code still render as escaped text', () => {
 assert(render('```\n<img src=x>').includes('&lt;img src=x&gt;'));
 assert(render('```\n'+'a'.repeat(1900)+'\n```').includes('a'.repeat(1900)));
});

test('Enter recognizes code fences at caret, including nested runs and tilde fences', () => {
 const inside=['```','```go\nline','~~~python\nline','````\n```\nline','```\n~~~\nline','  ```js\nline'];
 for(const text of inside)assert.equal(insideCodeFence(text,text.length),true,text);
 const outside=['plain','text `inline`','```\ncode\n```','~~~\nx\n~~~~','```bad`info','text ```'];
 for(const text of outside)assert.equal(insideCodeFence(text,text.length),false,text);
 const whole='before\n```go\ncode\n```\nafter';
 assert(insideCodeFence(whole,whole.indexOf('code')+2));
 assert(!insideCodeFence(whole,3));
});
