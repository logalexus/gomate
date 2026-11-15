// Real WebGL scene: triangle meshes, perspective camera, depth testing and lighting.
// No external assets or runtime downloads; the game remains a lazy-loaded module.
const COLORS = [[.35,.95,.77],[1,.61,.29],[.65,.53,1],[1,.35,.57]];
const vertexSource = `
attribute vec3 aPosition;
attribute vec3 aNormal;
uniform mat4 uProjection, uView, uModel;
varying mediump vec3 vWorld, vNormal;
void main() {
 vec4 world = uModel * vec4(aPosition, 1.0);
 vWorld = world.xyz;
 vNormal = normalize(mat3(uModel) * aNormal);
 gl_Position = uProjection * uView * world;
}`;
const fragmentSource = `
precision mediump float;
uniform vec3 uColor, uCamera;
uniform float uEmission, uGrid;
varying mediump vec3 vWorld, vNormal;
void main() {
 float light = .32 + .68 * max(0.0, dot(normalize(vNormal), normalize(vec3(-.4, 1., .3))));
 vec3 color = uColor * mix(light, 1., uEmission);
 if (uGrid > .5) {
  vec2 cell = fract(vWorld.xz);
  float line = 1. - step(.018, min(min(cell.x, 1.-cell.x), min(cell.y, 1.-cell.y)));
  color += vec3(.015, .06, .075) * line;
 }
 float fog = smoothstep(10., 32., distance(uCamera, vWorld));
 gl_FragColor = vec4(mix(color, vec3(.025, .04, .07), fog), 1.);
}`;
function perspective(aspect) {
 const f = 1 / Math.tan(Math.PI * 63 / 360), near = .04, far = 60;
 return new Float32Array([f/aspect,0,0,0, 0,f,0,0, 0,0,(far+near)/(near-far),-1, 0,0,2*far*near/(near-far),0]);
}
function view(x,y,z,angle,pitch) {
 const c=Math.cos(angle),s=Math.sin(angle),cp=Math.cos(pitch),sp=Math.sin(pitch);
 const r=[-s,0,c],u=[-c*sp,cp,-s*sp],f=[c*cp,sp,s*cp];
 return new Float32Array([r[0],u[0],-f[0],0, r[1],u[1],-f[1],0, r[2],u[2],-f[2],0,
  -(r[0]*x+r[1]*y+r[2]*z),-(u[0]*x+u[1]*y+u[2]*z),f[0]*x+f[1]*y+f[2]*z,1]);
}
const identity=new Float32Array([1,0,0,0,0,1,0,0,0,0,1,0,0,0,0,1]);
function cubeVertices() {
 const result=[];
 const faces=[
  [[1,0,0],[[.5,-.5,-.5],[.5,.5,-.5],[.5,.5,.5],[.5,-.5,.5]]],
  [[-1,0,0],[[-.5,-.5,.5],[-.5,.5,.5],[-.5,.5,-.5],[-.5,-.5,-.5]]],
  [[0,1,0],[[-.5,.5,-.5],[-.5,.5,.5],[.5,.5,.5],[.5,.5,-.5]]],
  [[0,-1,0],[[-.5,-.5,.5],[-.5,-.5,-.5],[.5,-.5,-.5],[.5,-.5,.5]]],
  [[0,0,1],[[.5,-.5,.5],[.5,.5,.5],[-.5,.5,.5],[-.5,-.5,.5]]],
  [[0,0,-1],[[-.5,-.5,-.5],[-.5,.5,-.5],[.5,.5,-.5],[.5,-.5,-.5]]]
 ];
 for(const [normal,points] of faces) for(const i of [0,1,2,0,2,3]) result.push(...points[i],...normal);
 return new Float32Array(result);
}
export class ArenaRenderer {
 constructor(canvas) {
  this.canvas=canvas;
  const gl=canvas.getContext('webgl',{alpha:false,antialias:false,powerPreference:'low-power'});
  if(!gl) throw Error('The 3D arena requires WebGL. Enable hardware acceleration in your browser.');
  this.gl=gl;this.camera=null;this.lastShot=0;this.flashUntil=0;this.model=new Float32Array(16);
  this.reduced=matchMedia('(prefers-reduced-motion: reduce)').matches;
  const shader=(type,source)=>{const s=gl.createShader(type);gl.shaderSource(s,source);gl.compileShader(s);if(!gl.getShaderParameter(s,gl.COMPILE_STATUS)){const error=gl.getShaderInfoLog(s);gl.deleteShader(s);throw Error(error)}return s};
  const vs=shader(gl.VERTEX_SHADER,vertexSource),fs=shader(gl.FRAGMENT_SHADER,fragmentSource);
  this.program=gl.createProgram();gl.attachShader(this.program,vs);gl.attachShader(this.program,fs);gl.linkProgram(this.program);gl.deleteShader(vs);gl.deleteShader(fs);
  if(!gl.getProgramParameter(this.program,gl.LINK_STATUS))throw Error('Could not start the 3D renderer.');
  gl.useProgram(this.program);this.uniform={};
  for(const name of ['Projection','View','Model','Color','Camera','Emission','Grid'])this.uniform[name]=gl.getUniformLocation(this.program,'u'+name);
  this.buffer=gl.createBuffer();gl.bindBuffer(gl.ARRAY_BUFFER,this.buffer);gl.bufferData(gl.ARRAY_BUFFER,cubeVertices(),gl.STATIC_DRAW);
  for(const [name,offset] of [['aPosition',0],['aNormal',12]]){const loc=gl.getAttribLocation(this.program,name);gl.enableVertexAttribArray(loc);gl.vertexAttribPointer(loc,3,gl.FLOAT,false,24,offset)}
  gl.enable(gl.DEPTH_TEST);gl.depthFunc(gl.LEQUAL);gl.clearColor(.025,.04,.07,1);
 }
 box(x,y,z,sx,sy,sz,color,yaw=0,pitch=0,emission=0,grid=0) {
  const gl=this.gl,m=this.model,c=Math.cos(yaw),s=Math.sin(yaw),cp=Math.cos(pitch),sp=Math.sin(pitch);
  m.set([c*sx,0,-s*sx,0, s*sp*sy,cp*sy,c*sp*sy,0, s*cp*sz,-sp*sz,c*cp*sz,0, x,y,z,1]);
  gl.uniformMatrix4fv(this.uniform.Model,false,m);gl.uniform3fv(this.uniform.Color,color);
  gl.uniform1f(this.uniform.Emission,emission);gl.uniform1f(this.uniform.Grid,grid);gl.drawArrays(gl.TRIANGLES,0,36);
 }
 draw(map,players,id,angle,pitch,dt,now) {
  const gl=this.gl,w=this.canvas.width,h=this.canvas.height,me=players.find(p=>p.id===id);
  if(!map||!me||gl.isContextLost())return;
  if(!this.camera||Math.hypot(this.camera.x-me.x,this.camera.y-me.y)>2)this.camera={x:me.x,y:me.y};
  const smooth=1-Math.exp(-dt*22);this.camera.x+=(me.x-this.camera.x)*smooth;this.camera.y+=(me.y-this.camera.y)*smooth;
  const x=this.camera.x,z=this.camera.y,eye=me.hp>0?1.25:.45;
  gl.viewport(0,0,w,h);gl.clear(gl.COLOR_BUFFER_BIT|gl.DEPTH_BUFFER_BIT);
  gl.uniformMatrix4fv(this.uniform.Projection,false,perspective(w/h));gl.uniformMatrix4fv(this.uniform.View,false,view(x,eye,z,angle,pitch));gl.uniform3f(this.uniform.Camera,x,eye,z);
  this.box(8,-.1,7,16,.2,14,[.09,.13,.18],0,0,0,1);
  for(let iy=0;iy<map.length;iy++)for(let ix=0;ix<map[iy].length;ix++){
   const tile=map[iy][ix];if(tile==='0')continue;
   const color=tile==='2'?[.13,.32,.36]:tile==='3'?[.27,.19,.39]:[.13,.19,.26];
   this.box(ix+.5,1.25,iy+.5,.99,2.5,.99,color);
   this.box(ix+.5,2.51,iy+.5,1.01,.08,1.01,[.22,.29,.36]);
   this.box(ix+.5,.22,iy+.5,1.01,.11,1.01,[.04,.07,.11]);
   this.box(ix+.5,1.95,iy+.5,1.012,.045,1.012,tile==='3'?[.67,.48,1]:[.24,.8,.77],0,0,1);
  }
  // Volumetric central fixtures give the room visible height and overhead geometry.
  this.box(8,3.7,7,.22,.2,12,[.16,.23,.3]);
  this.box(8,3.58,7,.08,.04,10,[.4,.85,1],0,0,1);
  for(const p of players){
   if(p.id===id||p.hp<=0)continue;
   const yaw=Math.PI/2-p.angle,color=COLORS[p.id%COLORS.length];
   const part=(lx,ly,lz,sx,sy,sz,c,emission=0)=>this.box(p.x+Math.cos(yaw)*lx+Math.sin(yaw)*lz,ly,p.y-Math.sin(yaw)*lx+Math.cos(yaw)*lz,sx,sy,sz,c,yaw,0,emission);
   part(-.14,.32,0,.17,.64,.24,[.1,.15,.21]);part(.14,.32,0,.17,.64,.24,[.1,.15,.21]);
   part(0,.94,0,.48,.58,.3,color);part(0,1.42,0,.37,.34,.34,[.16,.23,.29]);
   part(0,1.44,.18,.3,.075,.035,p.shieldUntil>Date.now()?[.8,1,1]:color,1);
   part(-.32,.96,0,.13,.5,.17,color);part(.32,.96,.04,.13,.5,.17,color);
   this.box(p.x+Math.cos(p.angle)*.38,1.13,p.y+Math.sin(p.angle)*.38,.14,.13,.5,[.19,.25,.3],yaw,-(p.pitch||0));
  }
  // The weapon is itself a set of 3D meshes in camera space, with its own depth pass.
  if(me.hp>0){
   if(me.shot!==this.lastShot){this.lastShot=me.shot;this.flashUntil=now+90}
   const recoil=!this.reduced&&now<this.flashUntil?.04:0;
   gl.clear(gl.DEPTH_BUFFER_BIT);gl.uniformMatrix4fv(this.uniform.View,false,identity);gl.uniform3f(this.uniform.Camera,0,0,0);
   this.box(.23,-.28+recoil,-.48,.12,.14,.38,[.2,.28,.36]);
   this.box(.23,-.37+recoil,-.35,.09,.19,.12,[.08,.12,.17],0,-.2);
   this.box(.23,-.255+recoil,-.74,.065,.07,.18,[.1,.15,.2]);
   this.box(.23,-.202+recoil,-.55,.045,.015,.2,[.35,.96,.75],0,0,1);
   this.box(.23,-.19+recoil,-.68,.04,.045,.04,[.08,.12,.17]);
   if(now<this.flashUntil&&!this.reduced)this.box(.23,-.255+recoil,-.86,.09,.09,.12,[1,.89,.46],0,Math.PI/4,1);
  }
 }
 dispose(){const gl=this.gl;gl.deleteBuffer(this.buffer);gl.deleteProgram(this.program)}
}
