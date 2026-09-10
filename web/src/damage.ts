import * as THREE from 'three';
import type { EntityView } from './api.generated';

// Damage stages come from Go. These meshes only describe observed condition:
// no fire damage, movement penalty, predicted death, or collision authority.
const chip = new THREE.DodecahedronGeometry(1, 0);
const flame = new THREE.ConeGeometry(1, 1, 5);
const block = new THREE.BoxGeometry(1, 1, 1);
const soot = new THREE.MeshStandardMaterial({color:'#39372e',roughness:1,flatShading:true});
const rubble = new THREE.MeshStandardMaterial({color:'#817764',roughness:1,flatShading:true});
const ember = new THREE.MeshBasicMaterial({color:'#ef762b'});
const hot = new THREE.MeshBasicMaterial({color:'#ffcf68'});
const smoke = new THREE.MeshBasicMaterial({color:'#5c5950',transparent:true,opacity:.5,depthWrite:false});
const bandage = new THREE.MeshStandardMaterial({color:'#c9bda3',roughness:1});
const charredTimber = new THREE.MeshStandardMaterial({color:'#554332',roughness:1,flatShading:true});
const naval = new Set(['galley','fire_ship','fishing_ship','transport','trade_ship']);
const wagons = new Set(['trade_cart','supply_cart','ram','mangonel','trebuchet','bombard_cannon']);

export class DamagePlume extends THREE.Group {
  private outer:THREE.InstancedMesh;
  private inner:THREE.InstancedMesh;
  private clouds:THREE.InstancedMesh;
  private dummy=new THREE.Object3D();
  constructor(private radius:number,private height:number,private seed:number,private intensity:number) {
    super();
    this.outer=new THREE.InstancedMesh(flame,ember,6);
    this.inner=new THREE.InstancedMesh(flame,hot,6);
    this.clouds=new THREE.InstancedMesh(chip,smoke,5);
    for(const mesh of [this.outer,this.inner,this.clouds]) {mesh.frustumCulled=false;this.add(mesh)}
    this.update(0,true);
  }
  update(time:number,reduced:boolean,collapse=0) {
    const t=reduced?0:time;
    const height=this.height*(1-collapse)+.1*collapse;
    for(let i=0;i<6;i++) {
      const a=this.seed+i*2.4, width=this.radius*(.14+.035*(i%3)), pulse=1+Math.sin(t*7+i*1.7)*.18;
      const h=(.5+this.intensity*.22+(i%3)*.12)*pulse;
      this.dummy.position.set(Math.cos(a)*this.radius*.53,height+h*.35,Math.sin(a)*this.radius*.53);
      this.dummy.rotation.set(Math.sin(t*2+i)*.12,0,Math.cos(t*3+i)*.16);
      this.dummy.scale.set(width,h,width*.85);this.dummy.updateMatrix();this.outer.setMatrixAt(i,this.dummy.matrix);
      this.dummy.position.y-=h*.16;this.dummy.scale.multiplyScalar(.56);this.dummy.updateMatrix();this.inner.setMatrixAt(i,this.dummy.matrix);
    }
    for(let i=0;i<5;i++) {
      const life=(t*.27+i*.2)%1, a=this.seed+i*2.4;
      this.dummy.position.set(Math.cos(a)*this.radius*.3+life*.6,height+.6+life*2.7,Math.sin(a)*this.radius*.3);
      this.dummy.rotation.set(i*.6,life,i*.3);this.dummy.scale.setScalar((.2+life*.5)*this.radius);
      this.dummy.updateMatrix();this.clouds.setMatrixAt(i,this.dummy.matrix);
    }
    this.outer.instanceMatrix.needsUpdate=this.inner.instanceMatrix.needsUpdate=this.clouds.instanceMatrix.needsUpdate=true;
  }
}

export function decorateDamage(g:THREE.Group,e:EntityView,roof:THREE.BufferGeometry) {
  const stage=e.damage_stage??0;
  if(!stage || !['building','unit'].includes(e.kind))return;
  const originals=new Map<THREE.Material,THREE.Material>();
  const building=e.kind==='building';
  g.traverse(o=>{
    if(!(o instanceof THREE.Mesh))return;
    const source=o.material as THREE.MeshStandardMaterial;
    let material=originals.get(source);
    if(!material) {const copy=source.clone();copy.color.multiplyScalar(1-stage*(building?.16:.09));copy.roughness=1;material=copy;originals.set(source,material)}
    o.material=material;o.userData.privateMaterial=true;
    // Open missing roof faces, exposing the dark interior, instead of only
    // recoloring an otherwise perfect building silhouette.
    if(building && stage>=2 && o.geometry===roof) {
      const geometry=o.geometry.clone(),index=geometry.getIndex();
      if(index){const kept:number[]=[];for(let i=0;i<index.count;i+=3)if((i/3+e.id)%5>=stage-1)kept.push(index.getX(i),index.getX(i+1),index.getX(i+2));geometry.setIndex(kept)}
      o.geometry=geometry;o.userData.privateGeometry=true;
      if(stage===3){o.rotation.z=.12;o.position.y-=.12}
    }
  });
  if(building) {
    for(let i=0;i<stage*4;i++) {
      const a=i*2.4+e.id, r=e.radius*(.76+(i%3)*.12), mesh=new THREE.Mesh(chip,i%3? rubble:soot);
      mesh.position.set(Math.cos(a)*r,.1+(i%2)*.07,Math.sin(a)*r);
      mesh.rotation.set(i*.3,i*.7,.4);mesh.scale.set(.12+(i%3)*.08,.11+(i%2)*.06,.2);mesh.castShadow=true;g.add(mesh);
    }
    // Fractures on all elevations remain readable when the camera rotates.
    for(let face=0;face<4;face++)for(let n=0;n<stage+1;n++) {
      const mark=new THREE.Mesh(block,soot),angle=face*Math.PI/2;
      const x=(n%2?.1:-.04)*e.radius,y=.45+n*.2;
      mark.position.set(Math.sin(angle)*e.radius*.83+Math.cos(angle)*x,y,Math.cos(angle)*e.radius*.83-Math.sin(angle)*x);
      mark.scale.set(.035,.3,.02);mark.rotation.set(0,angle,n%2?.3:-.4);g.add(mark);
    }
  } else if(stage>=2) {
    g.rotation.z=(e.id%2?1:-1)*(naval.has(e.type)?.08:wagons.has(e.type)?.1:.12);
    if(!naval.has(e.type)&&!wagons.has(e.type)) {const wrap=new THREE.Mesh(block,bandage);wrap.position.set(0,.46, .115);wrap.scale.set(.28,.06,.025);wrap.rotation.z=-.25;g.add(wrap)}
  }
  if(stage>=2&&(building||naval.has(e.type)||wagons.has(e.type))) {
    // Place flames in the upper structure: footprint alone puts a small
    // building's fire beneath its roof and hides the damage at normal zoom.
    const height=building?new THREE.Box3().setFromObject(g).max.y*.68:.55;
    const plume=new DamagePlume(Math.max(.5,e.radius),height,e.id,stage);
    g.add(plume);g.userData.damagePlume=plume;
  }
}

export function woundStride(e:EntityView,side:number,time:number) {
  const stage=e.damage_stage??0, wounded=stage>=2;
  return Math.sin(time*(wounded?8:12)+side*10)*(wounded&&side>0?.11:.35);
}

export function makeRubble(radius:number,seed:number) {
  const group=new THREE.Group(),dummy=new THREE.Object3D();
  const scar=new THREE.Mesh(block,soot);
  scar.scale.set(radius*1.8,.035,radius*1.8);scar.position.y=.03;scar.receiveShadow=true;group.add(scar);
  const count=18+Math.ceil(radius*4),burntCount=Math.ceil(count/4);
  const burnt=new THREE.InstancedMesh(chip,soot,burntCount),stones=new THREE.InstancedMesh(chip,rubble,count-burntCount),beams=new THREE.InstancedMesh(block,charredTimber,5);
  for(const mesh of [burnt,stones,beams]){mesh.castShadow=mesh.receiveShadow=true;group.add(mesh)}
  let normal=0,charred=0;
  for(let i=0;i<count;i++) {
    const a=seed+i*2.4,d=radius*(.12+(i%5)*.18),size=.12+(i%4)*.07;
    dummy.position.set(Math.cos(a)*d,.08+size*.6,Math.sin(a)*d);
    dummy.rotation.set(i*.7,a,i*.35);dummy.scale.set(size*1.3,size,size);dummy.updateMatrix();
    if(i%4)stones.setMatrixAt(normal++,dummy.matrix);else burnt.setMatrixAt(charred++,dummy.matrix);
  }
  for(let i=0;i<5;i++) {
    const a=seed+i*2.4;
    dummy.position.set(Math.cos(a)*radius*.35,.15+i*.025,Math.sin(a)*radius*.35);
    dummy.rotation.set(.07,a,.1);dummy.scale.set(.12,.13,radius*(.75+(i%2)*.3));dummy.updateMatrix();beams.setMatrixAt(i,dummy.matrix);
  }
  return group;
}
