import * as THREE from 'three';
import type { EntityView } from './api.generated';

// THESIS: buildings read as individual crafts, and a settlement visibly matures.
// OWN-WORLD: original timber, daub, dressed stone, clay, slate and woven thatch.
// STORY: identify a building from its silhouette and material at any age.
// FIRST VIEWPORT: textured roofs and walls sit beneath the existing player flags.
// FORM: extend the procedural miniature; Go supplies the observed owner's age.
type Surface = 'wall' | 'roof' | 'timber' | 'stone' | 'soil';
type Style = { wall: string; roof: string; timber: string; courses: number; bond: number };
const styles: Record<string, Style> = {
  town_center: { wall:'#d2c6a4', roof:'#a85c40', timber:'#68472f', courses:6, bond:0 },
  house: { wall:'#d8cbb0', roof:'#97523b', timber:'#806043', courses:5, bond:1 },
  mill: { wall:'#c8c2a1', roof:'#717950', timber:'#786344', courses:7, bond:2 },
  lumber_camp: { wall:'#b99a68', roof:'#846139', timber:'#835e37', courses:4, bond:3 },
  mining_camp: { wall:'#b0afa0', roof:'#676976', timber:'#66553e', courses:8, bond:4 },
  farm: { wall:'#ad9663', roof:'#b69a57', timber:'#786644', courses:9, bond:5 },
  barracks: { wall:'#b7a78e', roof:'#824333', timber:'#604b38', courses:6, bond:6 },
  archery_range: { wall:'#c6bc99', roof:'#596f50', timber:'#79623b', courses:7, bond:7 },
  stable: { wall:'#c5ab86', roof:'#a98248', timber:'#6a4a2c', courses:5, bond:8 },
  blacksmith: { wall:'#aaa195', roof:'#505655', timber:'#514233', courses:9, bond:9 },
  market: { wall:'#d7bb96', roof:'#b77c4b', timber:'#89613f', courses:6, bond:10 },
  tower: { wall:'#b5b5a4', roof:'#667078', timber:'#5d5040', courses:10, bond:11 },
  wall: { wall:'#a9aba3', roof:'#676960', timber:'#71654b', courses:5, bond:12 },
  gate: { wall:'#c4bda9', roof:'#776b51', timber:'#6b4931', courses:8, bond:13 },
  palisade: { wall:'#a58b5d', roof:'#8f7444', timber:'#957347', courses:4, bond:14 },
  castle: { wall:'#a8b1b5', roof:'#556672', timber:'#57493c', courses:9, bond:15 },
  siege_workshop: { wall:'#b5a083', roof:'#776049', timber:'#635039', courses:7, bond:16 },
  monastery: { wall:'#d9d4bd', roof:'#6d7e79', timber:'#65583f', courses:8, bond:17 },
  university: { wall:'#c7b9a9', roof:'#785d79', timber:'#70573e', courses:10, bond:18 },
  dock: { wall:'#bcb596', roof:'#527e80', timber:'#948267', courses:6, bond:19 },
  wonder: { wall:'#e1d5b5', roof:'#b49652', timber:'#795b35', courses:12, bond:20 },
};
const cache = new Map<string, THREE.MeshStandardMaterial>();
const size = 128;
const channels = (hex: string) => [1,3,5].map(i => parseInt(hex.slice(i,i+2),16));
const mix = (a:number[],b:number[],weight:number) => a.map((v,i)=>v*(1-weight)+b[i]*weight);
const noise = (x:number,y:number,seed:number) => { const n=Math.sin(x*127.1+y*311.7+seed*74.7)*43758.5453;return n-Math.floor(n); };

function texture(style:Style, surface:Surface, age:number) {
  const pixels = new Uint8Array(size*size*4), seed = style.bond;
  const base = channels(surface==='roof' ? style.roof : surface==='timber' ? style.timber : surface==='soil' ? '#786044' : style.wall);
  const thatch = surface==='roof' && age===0;
  const color = thatch ? mix(base,channels('#bba15e'),.6) : surface==='wall' && age===0 ? mix(base,channels('#ac9671'),.45) : age===3 && surface!=='soil' ? mix(base,channels('#eee2c6'),.09) : base;
  for(let y=0;y<size;y++)for(let x=0;x<size;x++){
    let shade=1, line=false;
    const grit=noise(x,y,seed)-.5;
    if(surface==='timber'){
      const plank=16+(seed%3)*8, grain=Math.sin(x*.75+Math.sin(y*.06+seed)*2);
      line=x%plank<2;shade=.9+grain*.065+noise(Math.floor(x/plank),0,seed)*.16;
      if(age>1&&y%48<2)shade*=.72;
    }else if(surface==='soil'){
      const furrow=12+age*2;shade=.82+.22*Math.sin((x+y*(age===0?.13:0))*Math.PI*2/furrow);
      if(age===3&&y%32<3)shade*=.7;
    }else if(thatch){
      shade=.82+.24*noise(Math.floor(x/2),Math.floor(y/32),seed);
      line=y%32<3;shade+=Math.sin(y*.35+x)*.045;
    }else if(surface==='roof'){
      const h=age===1?16:12,w=age===3?16:24,row=Math.floor(y/h);
      line=y%h<2||(x+(row%2)*w/2+seed)%w<1;
      shade=.85+.18*noise(Math.floor((x+row%2*w/2)/w),row,seed)+.12*(y%h)/h;
      if(age===3&&(y+seed*3)%48<3)shade*=1.18;
    }else if(surface==='wall'&&age<2){
      shade=1+grit*.09;
      if(age===0){line=y%32<2;shade+=Math.sin(x*.7)*.025;}
      else { const beam=x%64<4||y%64<4; if(beam)shade=.54; }
    }else{
      const rows=age===0?Math.max(3,Math.floor(style.courses*.55)):age===1?Math.max(4,Math.floor(style.courses*.8)):style.courses+(age===3?2:0);
      const h=Math.floor(size/rows),row=Math.floor(y/h),w=[48,40,32,24][age];
      const offset=(row%2)*w/2+seed*3;
      line=y%h<2||(x+offset)%w<2;
      shade=.88+noise(Math.floor((x+offset)/w),row,seed)*.2;
      if(age===3&&y<5)shade=1.14;
    }
    if(line)shade*=surface==='wall'||surface==='stone'?.65:.7;
    shade+=grit*(age===0?.11:.055);
    const i=(y*size+x)*4;
    for(let c=0;c<3;c++)pixels[i+c]=Math.max(0,Math.min(255,Math.round(color[c]*shade)));
    pixels[i+3]=255;
  }
  const map=new THREE.DataTexture(pixels,size,size,THREE.RGBAFormat);
  map.colorSpace=THREE.SRGBColorSpace;map.wrapS=map.wrapT=THREE.RepeatWrapping;
  map.magFilter=THREE.LinearFilter;map.minFilter=THREE.LinearMipmapLinearFilter;map.generateMipmaps=true;map.anisotropy=4;map.needsUpdate=true;
  return map;
}

function material(type:string,surface:Surface,age:number){
  const key=`${type}:${age}:${surface}`;
  let m=cache.get(key);
  if(!m){
    const map=texture(styles[type],surface,age);
    m=new THREE.MeshStandardMaterial({map,bumpMap:map,bumpScale:surface==='roof'?.045:.025,roughness:age===3&&surface==='roof'?.65:.94,flatShading:true});
    m.name=`building:${key}`;m.userData={building:type,appearanceAge:age,surface};cache.set(key,m);
  }
  return m;
}

export function dressBuilding(group:THREE.Group,entity:EntityView){
  if(!Object.hasOwn(styles,entity.type))return;
  const age=Math.min(3,Math.max(0,entity.appearance_age??0));
  const surfaces:Record<string,Surface>={'d6cfb4':'wall','acaa95':'stone','9e5843':'roof','6e4f36':'timber','817962':'stone','9b815b':'timber','756144':'soil'};
  group.traverse(object=>{
    if(!(object instanceof THREE.Mesh)||object instanceof THREE.InstancedMesh)return;
    const source=object.material as THREE.MeshStandardMaterial,surface=surfaces[source.color.getHexString()];
    if(surface)object.material=material(entity.type,surface,age);
  });
}

// Materials are shared by type/age, never by entity or owner. The cache has a
// fixed 21 × 4 × 5 upper bound, and old age variants are reused after reconnect.
