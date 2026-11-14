// Small software 3D raycaster. No assets, WebGL context or game libraries in the chat bundle.
const colors=['#73fbd3','#ffb86b','#a99bff','#ff7caa'];
const wrap=a=>Math.atan2(Math.sin(a),Math.cos(a));
export class ArenaRenderer {
 constructor(canvas){this.canvas=canvas;this.ctx=canvas.getContext('2d',{alpha:false});this.camera=null;this.sprites=colors.map(c=>this.robot(c));this.depth=[];this.lastShot=0;this.flashUntil=0;this.lastHP=100;this.hurtUntil=0;this.reduced=matchMedia('(prefers-reduced-motion: reduce)').matches}
 robot(color){const c=document.createElement('canvas');c.width=64;c.height=96;const g=c.getContext('2d');g.fillStyle='#080f1c';g.fillRect(12,25,40,48);g.fillStyle=color;g.fillRect(17,8,30,25);g.fillRect(15,37,34,31);g.fillRect(6,38,7,32);g.fillRect(51,38,7,32);g.fillStyle='#203246';g.fillRect(18,70,10,24);g.fillRect(36,70,10,24);g.fillStyle='#f0ffff';g.fillRect(22,17,20,5);g.fillStyle='#26384b';g.fillRect(28,40,8,22);g.fillStyle='#d5e6f2';g.fillRect(45,52,14,8);return c}
 draw(map,players,id,angle,dt,now){
  const c=this.canvas,g=this.ctx,w=c.width,h=c.height,me=players.find(p=>p.id===id);if(!map||!me)return;
  if(!this.camera||Math.hypot(this.camera.x-me.x,this.camera.y-me.y)>2)this.camera={x:me.x,y:me.y};
  const smooth=1-Math.exp(-dt*22);this.camera.x+=(me.x-this.camera.x)*smooth;this.camera.y+=(me.y-this.camera.y)*smooth;
  const {x,y}=this.camera,fov=Math.PI/2.8,plane=w/(2*Math.tan(fov/2));
  if(me.shot!==this.lastShot){this.lastShot=me.shot;this.flashUntil=now+90}if(me.hp<this.lastHP)this.hurtUntil=now+160;this.lastHP=me.hp;
  let gradient=g.createLinearGradient(0,0,0,h);gradient.addColorStop(0,'#080e1a');gradient.addColorStop(.5,'#22354b');gradient.addColorStop(.51,'#182331');gradient.addColorStop(1,'#090f1a');g.fillStyle=gradient;g.fillRect(0,0,w,h);
  // Floor perspective lines make depth and movement legible without expensive per-pixel textures.
  g.strokeStyle='#294355';g.lineWidth=1;g.beginPath();for(let d=1;d<22;d+=1){const sy=h/2+plane*.5/d;g.moveTo(0,sy);g.lineTo(w,sy)}for(let i=-8;i<=8;i++){g.moveTo(w/2+i*18,h/2);g.lineTo(w/2+i*w/5,h)}g.stroke();
  const stride=2;this.depth.length=Math.ceil(w/stride);
  for(let sx=0;sx<w;sx+=stride){
   const offset=Math.atan((sx-w/2)/plane),a=angle+offset,dx=Math.cos(a),dy=Math.sin(a);let mx=Math.floor(x),my=Math.floor(y);
   const ddx=Math.abs(1/dx),ddy=Math.abs(1/dy),stepX=dx<0?-1:1,stepY=dy<0?-1:1;
   let sideX=(dx<0?x-mx:mx+1-x)*ddx,sideY=(dy<0?y-my:my+1-y)*ddy,side=0,tile='1';
   for(let n=0;n<64;n++){if(sideX<sideY){sideX+=ddx;mx+=stepX;side=0}else{sideY+=ddy;my+=stepY;side=1}tile=map[my]?.[mx]||'1';if(tile!=='0')break}
   const distance=Math.max(.05,(side?sideY-ddy:sideX-ddx)*Math.cos(offset));this.depth[sx/stride]=distance;
   const height=plane/distance,top=(h-height)/2,texture=((side?x+(sideY-ddy)*dx:y+(sideX-ddx)*dy)%1+1)%1;
   const base=tile==='2'?[54,126,135]:tile==='3'?[102,75,145]:[53,74,96];let shade=Math.max(.18,1-distance/22)*(side?.72:1);if(texture<.035||texture>.965)shade*=.45;
   g.fillStyle=`rgb(${base.map(v=>Math.round(v*shade)).join(',')})`;g.fillRect(sx,top,stride,height);
   g.fillStyle=tile==='3'?'#b09bff':tile==='2'?'#71e6d2':'#4b879d';g.globalAlpha=Math.max(.15,1-distance/20);g.fillRect(sx,top+height*.13,stride,Math.max(1,height*.025));g.globalAlpha=1;
   g.fillStyle='#0c1725';g.fillRect(sx,top+height*.76,stride,Math.max(1,height*.015));
  }
  const enemies=players.filter(p=>p.id!==id&&p.hp>0).map(p=>{const a=wrap(Math.atan2(p.y-y,p.x-x)-angle);return {p,a,dist:Math.hypot(p.x-x,p.y-y)*Math.cos(a)}}).filter(v=>v.dist>.15&&Math.abs(v.a)<fov).sort((a,b)=>b.dist-a.dist);
  for(const {p,a,dist} of enemies){const height=plane*.85/dist,width=height*2/3,center=w/2+Math.tan(a)*plane,top=h/2+plane*.5/dist-height,left=center-width/2,sprite=this.sprites[p.id%4];
   for(let sx=Math.max(0,Math.floor(left/stride)*stride);sx<Math.min(w,left+width);sx+=stride){if(dist>=this.depth[Math.floor(sx/stride)])continue;const src=Math.max(0,Math.min(63,(sx-left)/width*64));g.drawImage(sprite,src,0,Math.min(64-src,64*stride/width),96,sx,top,stride,height)}
   if(center>0&&center<w&&dist<this.depth[Math.floor(center/stride)]){g.font='12px monospace';g.textAlign='center';g.fillStyle=colors[p.id%4];g.fillText(p.name,center,top-8);if(p.shieldUntil>Date.now()){g.strokeStyle='#c5eeff';g.strokeRect(left,top,width,height)}}
  }
  // View model, animated only on a confirmed server shot.
  const recoil=!this.reduced&&now<this.flashUntil?12:0,gx=w*.58,gy=h+recoil;
  g.fillStyle='#162536';g.beginPath();g.moveTo(gx-65,gy);g.lineTo(gx-26,gy-95);g.lineTo(gx+28,gy-95);g.lineTo(gx+78,gy);g.fill();g.fillStyle='#3c5368';g.fillRect(gx-19,gy-112,38,76);g.fillStyle='#78f4ce';g.fillRect(gx-11,gy-100,22,4);g.fillStyle='#0a111d';g.fillRect(gx-12,gy-120,24,18);
  if(now<this.flashUntil&&!this.reduced){g.fillStyle='#ddfff2';g.beginPath();g.moveTo(gx-15,gy-125);g.lineTo(gx-27,gy-155);g.lineTo(gx,gy-140);g.lineTo(gx+24,gy-155);g.lineTo(gx+14,gy-125);g.fill()}
  if(now<this.hurtUntil&&!this.reduced){g.strokeStyle='#ff587988';g.lineWidth=14;g.strokeRect(0,0,w,h)}
  g.strokeStyle='#e7fff5';g.lineWidth=1.5;g.beginPath();g.moveTo(w/2-9,h/2);g.lineTo(w/2-3,h/2);g.moveTo(w/2+3,h/2);g.lineTo(w/2+9,h/2);g.moveTo(w/2,h/2-9);g.lineTo(w/2,h/2-3);g.moveTo(w/2,h/2+3);g.lineTo(w/2,h/2+9);g.stroke();
  // A small minimap shows geometry and self only; opponents still need to be found.
  const scale=5,ox=w-16-map[0].length*scale,oy=16;g.fillStyle='#07101bdd';g.fillRect(ox-5,oy-5,map[0].length*scale+10,map.length*scale+10);
  map.forEach((row,iy)=>[...row].forEach((v,ix)=>{if(v!=='0'){g.fillStyle='#456077';g.fillRect(ox+ix*scale,oy+iy*scale,scale-1,scale-1)}}));g.fillStyle='#78f4ce';g.beginPath();g.arc(ox+x*scale,oy+y*scale,2,0,Math.PI*2);g.fill();g.strokeStyle='#78f4ce';g.beginPath();g.moveTo(ox+x*scale,oy+y*scale);g.lineTo(ox+(x+Math.cos(angle)*1.5)*scale,oy+(y+Math.sin(angle)*1.5)*scale);g.stroke();
 }
}
