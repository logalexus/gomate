<template><div class="script-editor"/></template>

<script>
import {Compartment, EditorState} from '@codemirror/state';
import {EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter} from '@codemirror/view';
import {defaultKeymap, history, historyKeymap} from '@codemirror/commands';
import {bracketMatching, indentOnInput, indentUnit, StreamLanguage, syntaxHighlighting, HighlightStyle} from '@codemirror/language';
import {python} from '@codemirror/lang-python';
import {shell} from '@codemirror/legacy-modes/mode/shell';
import {tags} from '@lezer/highlight';

const languageExtension = language => language === 'bash' ? StreamLanguage.define(shell) : python();
// Semantic tokens follow the application theme without rebuilding the editor or losing undo history.
const highlighting = HighlightStyle.define([
 {tag:tags.keyword, color:'var(--gm-info)', fontWeight:'600'},
 {tag:[tags.string, tags.special(tags.string)], color:'var(--gm-success)'},
 {tag:[tags.number, tags.bool, tags.null, tags.atom], color:'var(--gm-warning)'},
 {tag:tags.comment, color:'var(--gm-muted)', fontStyle:'italic'},
 {tag:[tags.function(tags.variableName), tags.function(tags.propertyName)], color:'var(--gm-accent)'},
 {tag:[tags.operator, tags.meta], color:'var(--gm-info)'},
 {tag:tags.invalid, color:'var(--gm-danger)', textDecoration:'underline'},
]);

export default {
 props:{value:{type:String,default:''},language:{type:String,default:'python'}},
 mounted(){
  this.languageSlot=new Compartment();
  this.editorView=new EditorView({parent:this.$el,state:EditorState.create({
   doc:this.value,
   extensions:[
    this.languageSlot.of(languageExtension(this.language)),
    lineNumbers(),highlightActiveLine(),highlightActiveLineGutter(),history(),
    indentOnInput(),indentUnit.of('    '),bracketMatching(),syntaxHighlighting(highlighting),
    // Tab remains normal focus navigation. Default keymap supports Enter indentation and Mod-[/].
    keymap.of([...defaultKeymap,...historyKeymap]),
    EditorView.contentAttributes.of({id:'request-script','aria-label':'Редактируемый скрипт','aria-multiline':'true',spellcheck:'false',autocapitalize:'off'}),
    EditorView.updateListener.of(update=>{if(update.docChanged)this.$emit('input',update.state.doc.toString())}),
   ],
  })});
 },
 watch:{
  value(value){const view=this.editorView;if(view&&value!==view.state.doc.toString())view.dispatch({changes:{from:0,to:view.state.doc.length,insert:value}})},
  language(value){if(this.editorView)this.editorView.dispatch({effects:this.languageSlot.reconfigure(languageExtension(value))})},
 },
 beforeDestroy(){this.editorView?.destroy();this.editorView=null},
 methods:{
  focus(){this.editorView?.focus()},
  copy(){
   // HTTP deployments lack navigator.clipboard. Copy via a temporary field inside the modal,
   // preserving the editor selection and scroll instead of selecting the entire document.
   const field=document.createElement('textarea'),previous=document.activeElement;
   field.value=this.value;field.setAttribute('aria-label','Копирование скрипта');
   Object.assign(field.style,{position:'fixed',left:'0',top:'0',width:'1px',height:'1px',opacity:'0'});
   this.$el.appendChild(field);
   try{field.focus({preventScroll:true});field.select();return document.execCommand('copy')}
   finally{field.remove();previous?.focus({preventScroll:true})}
  },
 },
};
</script>

<style>
.script-editor{min-width:0;min-height:0;height:100%;overflow:hidden;background:var(--gm-bg)}
.script-editor .cm-editor{height:100%;color:var(--gm-text);background:var(--gm-bg);font-size:13px}
.script-editor .cm-scroller{overflow:auto;font-family:var(--gm-mono);line-height:1.6}
.script-editor .cm-content{padding:10px 0;caret-color:var(--gm-text)}
.script-editor .cm-line{padding:0 12px}
.script-editor .cm-gutters{background:var(--gm-panel-alt);color:var(--gm-muted);border-right:1px solid var(--gm-border)}
.script-editor .cm-lineNumbers .cm-gutterElement{min-width:34px;padding:0 8px}
.script-editor .cm-activeLine,.script-editor .cm-activeLineGutter{background:var(--gm-accent-soft)}
.script-editor .cm-editor.cm-focused{outline:2px solid var(--gm-accent);outline-offset:-2px}
.script-editor .cm-content:focus-visible{outline:none!important}
.script-editor .cm-selectionBackground{background:var(--gm-accent-soft)!important}
.script-editor .cm-matchingBracket{background:var(--gm-accent-soft);outline:1px solid var(--gm-accent)}
</style>
