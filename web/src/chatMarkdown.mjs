import MarkdownIt from 'markdown-it';

// HTML is never accepted from messages. Keep remote images disabled: merely
// reading a team chat must not make requests to a sender-controlled server.
const markdown = new MarkdownIt({html: false, breaks: true, linkify: true});
markdown.disable('image');
const validateLink = markdown.validateLink.bind(markdown);
markdown.validateLink = url => /^(https?:|mailto:)/i.test(url) && validateLink(url);
markdown.renderer.rules.link_open = (tokens, index, options, env, renderer) => {
 tokens[index].attrSet('target', '_blank');
 tokens[index].attrSet('rel', 'noopener noreferrer');
 return renderer.renderToken(tokens, index, options);
};
markdown.renderer.rules.fence = (tokens, index) => {
 const token = tokens[index], escape = markdown.utils.escapeHtml;
 const language = token.info.trim().split(/\s+/)[0].slice(0, 40);
 return `<div class="chat-code"><div class="chat-code-toolbar"><span>${escape(language || 'Code')}</span><button type="button" class="chat-copy-code" aria-label="Copy code">Copy</button></div><pre><code>${escape(token.content)}</code></pre></div>\n`;
};

export function renderChatMarkdown(text) {
 return markdown.render(text);
}

// Enter inserts a newline while the caret is inside a fenced code block.
// Only a closing fence of the same character and at least the opening length
// closes the block. This also handles an unfinished draft.
export function insideCodeFence(text, caret) {
 let fence = null;
 for (const line of text.slice(0, caret).split('\n')) {
  if (fence) {
   const close = line.match(/^ {0,3}(`{3,}|~{3,})\s*$/);
   if (close && close[1][0] === fence[0] && close[1].length >= fence.length) fence = null;
  } else {
   const open = line.match(/^ {0,3}(`{3,}|~{3,})(.*)$/);
   if (open && (open[1][0] !== '`' || !open[2].includes('`'))) fence = open[1];
  }
 }
 return !!fence;
}
