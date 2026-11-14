<template>
 <article class="gm-packet" :class="{'packet-incoming':packet.incoming,'packet-outgoing':!packet.incoming}" :data-packet-id="packet.id">
  <header class="packet-heading">
   <div class="packet-identity"><span class="packet-direction"><i :class="packet.incoming?'fas fa-arrow-down':'fas fa-arrow-up'" aria-hidden="true"/>{{ packet.incoming?'Запрос':'Ответ' }}</span><span class="packet-number">#{{ packet.id }}</span><span class="packet-kind">{{ packet.kind||'raw' }}</span><span v-if="printPacketFlags(packet)" class="packet-flags">{{ printPacketFlags(packet) }}</span></div>
   <div class="packet-timing"><time>{{ dateToText(packet.timestamp) }}</time><span v-if="offset!==null">+{{ offset }} мс</span></div>
  </header>
  <div v-if="packet.metadata?.path||packet.metadata?.streamId||(packet.matches||[]).some(m=>m.crossBoundary)" class="packet-metadata"><span v-if="packet.metadata?.path">{{ packet.metadata.path }}</span><span v-if="packet.metadata?.streamId">RPC stream {{ packet.metadata.streamId }}</span><span v-if="(packet.matches||[]).some(m=>m.crossBoundary)" class="packet-cross-boundary">Совпадение продолжается из предыдущего блока</span></div>
  <p v-if="!$store.state.hexdumpMode" ref="payload" class="packet-content" v-html="stringdata"/>
  <p v-else class="packet-content packet-hex" v-html="hexdata"/>
  <footer class="packet-footer">
   <span class="packet-footnote">{{ atobLength<1024?atobLength+' Б':(atobLength/1024).toFixed(1)+' KiB' }}<span v-if="packet.incomplete"> · есть пропуски в захвате</span></span>
   <span v-if="copyState" class="packet-copy-status" role="status">{{ copyState }}</span>
   <b-dropdown :id="'packet-copy-'+packet.id" text="Копировать" size="sm" variant="outline-secondary" right><b-dropdown-item @click="copyText">Текст</b-dropdown-item><b-dropdown-item @click="copyRaw">HEX</b-dropdown-item><b-dropdown-item @click="copyPythonBytes">Python bytes</b-dropdown-item><template v-if="packet.incoming"><b-dropdown-divider/><b-dropdown-item-button @click="exportPacket('requests')">Python requests…</b-dropdown-item-button><b-dropdown-item-button @click="exportPacket('curl')">curl…</b-dropdown-item-button><b-dropdown-item-button @click="exportPacket('pwntools')">pwntools…</b-dropdown-item-button></template></b-dropdown>
   <button type="button" @mousedown.prevent @click="decodePacket" class="btn btn-sm btn-outline-primary"><i class="fas fa-code" aria-hidden="true"/> Декодировать</button>
  </footer>
 </article>
</template>

<!--suppress JSUnresolvedVariable, JSDeprecatedSymbols -->
<script>
	export default {
		name: 'Packet',
        data:()=>({copyState:''}),
		props: {
			packet: {
				id: Number(),
				matches: Array(),
				timestamp: Number(),
				incoming: Boolean(),
				ungzipped: Boolean(),
				webSocketParsed: Boolean(),
				tlsDecrypted: Boolean(),
				content: String(),
			},
			offset: Number(),
		},
		computed: {
            atobLength(){return atob(this.packet.content||'').length;},
			hexdata() {
				const dataString = atob(this.packet.content);
				return this.highlightHex(dataString);
			},
            stringdata() {
                return this.highlightPatterns(this.atou(this.packet.content));
            },
		},
		methods: {
            exportPacket(format){this.$emit('export',{packet:this.packet,format,returnFocus:this.$el.querySelector('.dropdown-toggle')})},
			decodePacket(event) {
				const selection=window.getSelection();
				const selected=selection&&!selection.isCollapsed&&this.$refs.payload?.contains(selection.anchorNode)&&this.$refs.payload?.contains(selection.focusNode)?selection.toString():'';
				const hex=this.$store.state.hexdumpMode&&!selected;
				const text=selected||(hex?Array.from(atob(this.packet.content),c=>c.charCodeAt(0).toString(16).padStart(2,'0')).join(''):this.atou(this.packet.content));
				this.$emit('decode',{text,source:`Блок #${this.packet.id} · ${selected?'выделение':hex?'исходные байты HEX':'полный текст'}`,operation:hex?'hex':null,returnFocus:event.currentTarget});
			},
			atou(b64) {
				const text = atob(b64);
				const length = text.length;
				const bytes = new Uint8Array(length);
				for (let i = 0; i < length; i++) {
					bytes[i] = text.charCodeAt(i);
				}
				const decoder = new TextDecoder();
				return decoder.decode(bytes);
			},
			printPacketFlags(packet) {
				let flags = [];
                if (packet.incomplete) flags.push('Неполный');

				if (packet.ungzipped) {
					flags.push('GZIP');
				}

				if (packet.webSocketParsed) {
					flags.push('WS');
				}

				if (packet.tlsDecrypted) {
					flags.push('TLS');
				}

				return flags.join(' ');
			},
            highlightHex(buffer) {
                const width = Math.max(1, Math.min(64, Number(this.$store.state.hexdumpBlockSize) || 16));
                const base = Math.max(2, Math.min(36, Number(this.$store.state.hexdumpLineNumberBase) || 10));
                const patterns = new Map(this.$store.state.patterns.map(p => [p.id,p]));
                const colors = new Map();
                for (const m of this.packet.matches || []) {
                    const p=patterns.get(m.patternId);
                    if (!p || p.deleted || p.searchType !== 'SUBBYTES' || !/^#[0-9a-f]{6}$/i.test(p.color)) continue;
                    for(let i=Math.max(0,m.startPosition);i<=Math.min(buffer.length-1,m.endPosition);i++) colors.set(i,p.color);
                }
                const lines=[];
                for(let start=0;start<buffer.length;start+=width) {
                    const block=buffer.slice(start,start+width);
                    let codes='';
                    for(let i=0;i<block.length;i++) {
                        const hex=block.charCodeAt(i).toString(16).padStart(2,'0').toUpperCase(), color=colors.get(start+i);
                        codes+=' '+(color ? `<span style="background-color:${color}">${hex}</span>` : hex);
                    }
                    const chars=this.escapeHtml(Array.from(block,c=>c.charCodeAt(0)<32 ? '.' : c).join(''));
                    lines.push(start.toString(base).padStart(10,'0')+':'+codes+' ..'.repeat(width-block.length)+' |'+chars+' '.repeat(width-block.length)+'|');
                }
                return lines.join('<br>');
            },

			hexdump(buffer, blockSize, lineNumberBase) {
				blockSize = parseInt(blockSize, 10) || 16;
				lineNumberBase = parseInt(lineNumberBase, 10) || 10;
				let lines = [];
				const hex = '0123456789ABCDEF';
				for (let b = 0; b < buffer.length; b += blockSize) {
					const block = buffer.slice(b, Math.min(b + blockSize, buffer.length));
					const addr = ('0000000000' + b.toString(lineNumberBase)).slice(-10);
					let codes = block.split('').map(ch => {
						const code = ch.charCodeAt(0);
						return ' ' + hex[(0xF0 & code) >> 4] + hex[0x0F & code];
					}).join('');
					codes += ' ..'.repeat(blockSize - block.length);
					// eslint-disable-next-line no-control-regex
					let chars = block.replace(/[\x00-\x1F]/g, '.');
					chars += ' '.repeat(blockSize - block.length);
					lines.push(addr + ':' + codes + ' |' + chars + '|');
				}
				return lines.join('\n');
			},
			dateToText(unixTimestamp) {
				return new Date(unixTimestamp).toLocaleDateString('ru-RU', {
					month: '2-digit',
					day: '2-digit',
					hour: '2-digit',
					minute: '2-digit',
					second: '2-digit',
				});
			},
            escapeHtml(value) {
                return value.replace(/[&<>"']/g, ch => ({'&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;'}[ch]));
            },
            highlightPatterns(raw) {
                const patterns = new Map(this.$store.state.patterns.map(p => [p.id, p]));
                const matches = (this.packet.matches || []).filter(m => {
                    const p = patterns.get(m.patternId);
                    return p && !p.deleted && p.searchType !== 'SUBBYTES' && /^#[0-9a-f]{6}$/i.test(p.color);
                }).sort((a, b) => a.startPosition - b.startPosition);
                let result = '', offset = 0;
                for (const match of matches) {
                    const start = match.startPosition, end = match.endPosition + 1;
                    if (start < offset || end > raw.length || end <= start) continue;
                    result += this.escapeHtml(raw.slice(offset, start));
                    result += `<span style="background-color: ${patterns.get(match.patternId).color}">${this.escapeHtml(raw.slice(start, end))}</span>`;
                    offset = end;
                }
                return result + this.escapeHtml(raw.slice(offset));
            },
			copyPythonBytes() {
				const data = 'b\'' + atob(this.packet.content)
					.split('')
					.map((aChar) => {
						return '\\x' + ('0' + aChar.charCodeAt(0).toString(16)).slice(-2);
					})
					.join('') + '\'';
				this.copyContent(data);
			},
			copyRaw() {
				this.copyContent(Array.from(atob(this.packet.content), c => c.charCodeAt(0).toString(16).padStart(2, '0')).join(''));
			},
			copyText() {
				this.copyContent(this.atou(this.packet.content));
			},
			copyContent(data) {
				const tempEl = document.createElement('textarea');
				tempEl.value = data; // Chrome
				tempEl.textContent = data; // Firefox
                const active=document.activeElement;tempEl.style.cssText='position:fixed;left:-9999px;top:0';
                document.body.appendChild(tempEl);
                try{tempEl.select();this.copyState=document.execCommand('copy')?'Скопировано':'Выделите текст и скопируйте вручную'}catch{this.copyState='Выделите текст и скопируйте вручную'}finally{tempEl.remove();active?.focus({preventScroll:true})}
			},
		},
	};
</script>

<style scoped>
.gm-packet{border:1px solid var(--gm-border);border-radius:var(--gm-radius);background:var(--gm-panel);overflow:visible;min-width:0;border-left:3px solid var(--gm-accent)}.gm-packet.packet-outgoing{border-left-color:var(--gm-warning)}.packet-heading{border-radius:calc(var(--gm-radius) - 1px) calc(var(--gm-radius) - 1px) 0 0;display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:7px;padding:6px 10px;border-bottom:1px solid var(--gm-border);background:var(--gm-panel-alt)}.packet-identity{display:flex;align-items:center;flex-wrap:wrap;gap:7px;font-size:.75rem}.packet-direction{display:flex;align-items:center;gap:7px;color:var(--gm-accent);font-weight:600}.packet-outgoing .packet-direction{color:var(--gm-warning)}.packet-number{font-family:var(--gm-mono);color:var(--gm-muted);font-size:.75rem}.packet-kind{border:1px solid var(--gm-border);border-radius:4px;font-family:var(--gm-mono);font-size:.75rem;padding:1px 5px;text-transform:uppercase}.packet-flags{color:var(--gm-warning);font-size:.75rem}.packet-timing{display:flex;gap:10px;color:var(--gm-muted);font-family:var(--gm-mono);font-size:.75rem}.packet-metadata{display:flex;flex-wrap:wrap;gap:8px;padding:5px 10px;font-family:var(--gm-mono);font-size:.75rem;border-bottom:1px solid var(--gm-border);overflow-wrap:anywhere;color:var(--gm-muted)}.packet-cross-boundary{color:var(--gm-warning)}.packet-content{margin:0;padding:10px;font-family:var(--gm-mono);font-size:.78rem;line-height:1.5;color:var(--gm-text);white-space:pre-wrap;overflow-wrap:anywhere;overflow:auto;max-height:440px;tab-size:4}.packet-hex{white-space:pre;overflow-wrap:normal;font-size:.75rem}.packet-footer{display:flex;align-items:center;flex-wrap:wrap;gap:7px;padding:5px 10px;border-top:1px solid var(--gm-border)}.packet-footnote{margin-right:auto;font-size:.75rem;color:var(--gm-muted)}.packet-copy-status{font-size:.75rem;color:var(--gm-success)}.packet-footer ::v-deep .btn{min-height:28px;padding:3px 9px;font-size:.75rem}.packet-footer .btn i{font-size:.75rem;margin-right:5px}@media(max-width:575px){.packet-heading{padding:6px 8px;gap:5px}.packet-content{padding:9px 8px}.packet-timing{gap:8px;font-size:.7rem}.packet-footer{padding:5px 8px}.packet-footnote{font-size:.7rem}.packet-metadata{padding:5px 8px}}
</style>
