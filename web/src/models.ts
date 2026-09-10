import * as THREE from 'three';
import { mergeGeometries } from 'three/addons/utils/BufferGeometryUtils.js';
import type { EntityView } from './api.generated';
import { dressBuilding } from './building-materials';
import { decorateDamage } from './damage';

const materials = new Map<string, THREE.MeshStandardMaterial>();
function material(color: string) {
  let m = materials.get(color);
  if (!m) { m = new THREE.MeshStandardMaterial({ color, roughness: .92, flatShading: true }); materials.set(color, m); }
  return m;
}
const cube = new THREE.BoxGeometry(1, 1, 1);
const cylinder = new THREE.CylinderGeometry(1, 1, 1, 8);
const cone = new THREE.ConeGeometry(1, 1, 4);
const roofGeometry = cone.clone().rotateY(Math.PI / 4);
const farmBed = new THREE.BoxGeometry(1, 1, 1, 12, 1, 12);
const pine = new THREE.ConeGeometry(1, 1, 7);
const stone = new THREE.DodecahedronGeometry(1, 0);
const orb = new THREE.SphereGeometry(1, 7, 5);
const ripple = new THREE.TorusGeometry(1, .035, 3, 20, Math.PI * 1.45);
const palette = { limestone: '#d6cfb4', shade: '#acaa95', roof: '#9e5843', wood: '#6e4f36', dark: '#384436', ground: '#a49870', metal: '#9ba2a0' };
export function ownerColor(owner: number) { return ['#b29b6f', '#6198c7', '#ba6756', '#bc9bd3', '#d5b459', '#65b4a1', '#df9470'][owner] ?? '#b29b6f'; }

function shape(group: THREE.Group, geometry: THREE.BufferGeometry, color: string, x: number, y: number, z: number, sx = 1, sy = 1, sz = 1) {
  const mesh = new THREE.Mesh(geometry, material(color));
  mesh.position.set(x, y, z); mesh.scale.set(sx, sy, sz); mesh.castShadow = true; mesh.receiveShadow = true;
  group.add(mesh); return mesh;
}
function box(g: THREE.Group, color: string, x: number, y: number, z: number, sx: number, sy: number, sz: number) { return shape(g, cube, color, x, y, z, sx, sy, sz); }
function roof(g: THREE.Group, x: number, y: number, z: number, width: number, height: number, depth: number) {
  box(g, '#654532', x, y - height / 2, z, width, .09, depth);
  shape(g, roofGeometry, palette.roof, x, y, z, width / Math.SQRT2, height, depth / Math.SQRT2);
}
function window(g: THREE.Group, x: number, y: number, z: number, side = false, size = .3) {
  box(g, palette.wood, x, y, z, side ? .085 : size + .1, size + .12, side ? size + .1 : .085);
  box(g, '#354843', x + (side ? .05 : 0), y, z + (side ? 0 : .05), side ? .02 : size, size, side ? size : .02);
  box(g, '#bca876', x + (side ? .067 : 0), y, z + (side ? 0 : .067), side ? .025 : .035, size, side ? .035 : .025);
  box(g, palette.limestone, x, y - size / 2 - .055, z, side ? .18 : size + .2, .075, side ? size + .2 : .18);
}
function timberWalls(g: THREE.Group, r: number, height: number) {
  for (const x of [-r * .81,r * .81]) for (const z of [-r * .76,r * .76]) box(g,palette.wood,x,height / 2+.12,z,.085,height,.085);
  for (const z of [-r * .78,r * .78]) box(g,palette.wood,0,height+.04,z,r*1.67,.1,.08);
  // The original fronts face north; the south and east elevations must also
  // read as buildings when the camera travels all the way around them.
  for (const x of [-r*.46,r*.46]) window(g,x,height*.63+.12,r*.785,false,r > 1 ? .32 : .23);
  for (const z of [-r*.37,r*.37]) window(g,r*.83,height*.63+.12,z,true,r > 1 ? .3 : .22);
}

function bakeStaticMeshes(g: THREE.Group) {
  // Detailed buildings stay cheap to draw. Animated parts retain their own
  // transforms, while static parts sharing a material become one mesh.
  const batches = new Map<THREE.Material, THREE.Mesh[]>();
  for (const o of g.children) {
    if (!(o instanceof THREE.Mesh) || o instanceof THREE.InstancedMesh || o.name) continue;
    const m = o.material as THREE.Material, batch = batches.get(m) ?? [];
    batch.push(o); batches.set(m,batch);
  }
  for (const [m,meshes] of batches) {
    if (meshes.length < 2) continue;
    const mixedIndices = meshes.some(mesh => !mesh.geometry.index) && meshes.some(mesh => !!mesh.geometry.index);
    const parts = meshes.map(mesh => {
      mesh.updateMatrix();
      // Rubble and wall fractures share a material but use different index
      // formats. Normalize this batch before merging their static geometry.
      const geometry = mixedIndices && mesh.geometry.index ? mesh.geometry.toNonIndexed() : mesh.geometry.clone();
      return geometry.applyMatrix4(mesh.matrix);
    });
    const geometry = mergeGeometries(parts);
    parts.forEach(part => part.dispose());
    if (!geometry) throw new Error('Unable to assemble battlefield geometry.');
    meshes.forEach(mesh => {g.remove(mesh);if(mesh.userData.privateGeometry)mesh.geometry.dispose()});
    const mesh = new THREE.Mesh(geometry,m); mesh.castShadow = true; mesh.receiveShadow = true; mesh.userData.privateGeometry = true; mesh.userData.privateMaterial = meshes.some(part=>part.userData.privateMaterial); g.add(mesh);
  }
}
function flag(g: THREE.Group, owner: number, x: number, y: number, z: number, scale = 1) {
  box(g, palette.wood, x, y + .8 * scale, z, .05, 1.6 * scale, .05);
  const f = box(g, ownerColor(owner), x + .28 * scale, y + 1.3 * scale, z, .55 * scale, .35 * scale, .035);
  f.name = 'flag';
}
function building(g: THREE.Group, e: EntityView) {
  const r = e.radius, type = e.type;
  if (type === 'farm') {
    shape(g, farmBed, '#756144', 0, .08, 0, r * 1.8, .12, r * 1.8);
    for (let row = -3; row <= 3; row++) box(g,'#8c7550',0,.155,row*.29,r*1.72,.055,.12);
    if ((e.amount ?? 0) <= 0) return;
    const crops = new THREE.InstancedMesh(pine, material('#c6b567'), 63);
    const matrix = new THREE.Matrix4(), rotation = new THREE.Quaternion();
    let index = 0;
    for (let i = -4; i <= 4; i++) for (let j = -3; j <= 3; j++) {
      matrix.compose(new THREE.Vector3(i * .24, .23, j * .29), rotation, new THREE.Vector3(.06, .35 + ((i + j) % 3) * .025, .06));
      crops.setMatrixAt(index++, matrix);
    }
    crops.castShadow = true; crops.receiveShadow = true; g.add(crops);
    return;
  }
  if (type === 'wall' || type === 'palisade' || type === 'gate') {
    const wood = type === 'palisade', color = wood ? palette.wood : palette.shade;
    if (type === 'gate') {
      const arch = new THREE.Group(); g.add(arch);
      // The observed connections determine its orientation on the server.
      if (e.connections?.some(d => d.y !== 0) && !e.connections.some(d => d.x !== 0)) arch.rotation.y = Math.PI / 2;
      for (const x of [-.54,.54]) box(arch, palette.shade, x,.75,0,.25,1.5,.48);
      box(arch,palette.limestone,0,1.52,0,1.35,.3,.5);
      for (let i=-2;i<=2;i++) box(arch,palette.wood,i*.16,1.12,0,.06,.6,.1);
      box(arch,palette.wood,0,1.12,0,.86,.07,.1);
    } else {
      box(g, color, 0, .6, 0, .65,1.2,.65);
      const links = e.connections?.length ? e.connections : [{x:1,y:0},{x:-1,y:0}];
      for (const d of links) box(g,color,d.x*.25,.6,d.y*.25,d.x ? .55 : .48,1.2,d.y ? .55 : .48);
      box(g,wood ? palette.wood : palette.limestone,0,1.3,0,.28,.35,.5);
      for (const d of links) box(g,wood ? palette.wood : palette.limestone,d.x*.43,1.3,d.y*.43,.22,.35,.35);
    }
    return;
  }
  box(g, '#817962', 0, .04, 0, r * 2, .16, r * 2);
  box(g, palette.shade, 0, .14, 0, r * 1.85, .16, r * 1.85);
  if (type === 'lumber_camp' || type === 'mining_camp') {
    for (const x of [-.8, .8]) for (const z of [-.6, .6]) box(g, palette.wood, x, .55, z, .1, 1.1, .1);
    roof(g, 0, 1.25, 0, 2.2, .65, 1.7);
    if (type === 'lumber_camp') for (let i = 0; i < 5; i++) {
      const log = shape(g, cylinder, '#876645', (i % 3) * .3 - .3, .25 + Math.floor(i / 3) * .22, 0, .13, 1.3, .13); log.rotation.x = Math.PI / 2;
    } else for (let i = 0; i < 4; i++) shape(g, stone, '#aba67f', i * .27 - .4, .3, 0, .25, .3, .22);
    return;
  }
  if (type === 'dock') {
    for (let i = 0; i < 8; i++) box(g, '#9b815b', i * .4 - 1.4, .18, 0, .37, .1, 3);
    for (const x of [-1.4, 1.4]) for (const z of [-1.3, 1.3]) shape(g, cylinder, palette.wood, x, .3, z, .09, 1, .09);
    roof(g, 0, 1.6, .6, 2, .8, 1.6); flag(g, e.owner, 1.3, .4, -1.2); return;
  }
  if (type === 'wonder') {
    for(let level=0;level<3;level++)box(g,palette.limestone,0,.25+level*.27,0,r*(1.9-level*.25),.3,r*(1.9-level*.25));
    box(g,palette.limestone,0,1.7,0,r*1.1,1.8,r*1.1);
    for(const x of [-r*.64,-r*.34,r*.34,r*.64])for(const z of [-r*.65,r*.65])shape(g,cylinder,palette.shade,x,1.6,z,.13,1.65,.13);
    roof(g,0,2.95,0,r*1.65,.9,r*1.65);shape(g,orb,'#c0a15c',0,3.55,0,.7,.6,.7);
    flag(g,e.owner,0,4.05,0,.6);return;
  }
  if (type === 'castle' || type === 'tower') {
    const h = type === 'tower' ? 2.5 : 2.15;
    box(g, palette.limestone, 0, h / 2, 0, r * 1.5, h, r * 1.5);
    const corners = type === 'tower' ? [[0, 0]] : [[-r * .7, -r * .7], [-r * .7, r * .7], [r * .7, -r * .7], [r * .7, r * .7]];
    for (const [x, z] of corners) {
      box(g, palette.shade, x, h * .58, z, .85, h * 1.16, .85);
      for (const dx of [-.3, .3]) for (const dz of [-.3, .3]) box(g, palette.limestone, x + dx, h * 1.22, z + dz, .23, .3, .23);
    }
    box(g, '#494234', 0, .55, -r * .76, .7, 1.1, .1);
    for (const side of [-1,1]) for (const offset of [-.28,.28]) {
      box(g,'#525746',offset,h*.66,side*r*.77,.1,.45,.045);
      box(g,'#525746',side*r*.77,h*.66,offset,.045,.45,.1);
    }
    flag(g, e.owner, 0, h + .15, 0, 1.2); return;
  }
  const height = type === 'town_center' ? 1.65 : type === 'house' ? 1.05 : 1.3;
  box(g, palette.limestone, 0, height / 2 + .12, 0, r * 1.65, height, r * 1.55);
  timberWalls(g,r,height);
  roof(g, 0, height + .57, 0, r * 1.95, .85, r * 1.85);
  for (const x of [-r * .77, r * .77]) box(g, palette.wood, x, height / 2, -r * .79, .09, height, .09);
  box(g, '#494032', 0, .45, -r * .79, .4, .75, .07);
  for (const x of [-r * .45, r * .45]) {
    box(g, '#44544d', x, height * .63, -r * .795, .25, .3, .08);
    box(g, palette.wood, x, height * .63, -r * .82, .035, .3, .08);
  }
  if (type === 'town_center') {
    box(g, palette.limestone, .9, 1.35, .5, 1.25, 2.7, 1.2);
    roof(g, .9, 3.2, .5, 1.6, 1, 1.6);
    window(g,.9,2.35,1.11,false,.38); window(g,1.535,2.35,.5,true,.34);
    for (let i = 0; i < 4; i++) box(g, palette.limestone, 0, .12 + i * .1, -1.8 + i * .2, 1.3, .2, .4);
    flag(g, e.owner, .9, 3.6, .5);
    box(g, ownerColor(e.owner), -.6, .98, -1.5, 1.9, .08, .85);
    for (const x of [-1.45, .2]) box(g, palette.wood, x, .5, -1.8, .07, 1, .07);
    box(g,'#574932',0,.55,r*.795,.5,.86,.09);
    box(g,palette.wood,0,.54,r*.85,.065,.82,.09);
    for (let i = 0; i < 3; i++) box(g,palette.shade,0,.07+i*.055,r*.98-i*.15,.9,.14,.3);
    box(g,ownerColor(e.owner),-.65,1.46,r*.82,.32,.63,.055);
  } else if (type === 'mill') {
    box(g, palette.shade, .4, 1.3, .2, .75, 2.5, .75);
    roof(g, .4, 2.8, .2, 1.1, .65, 1.1);
    const windmill = new THREE.Group(); windmill.name = 'windmill'; windmill.position.set(.4, 2.25, -.35);
    for (let i = 0; i < 4; i++) {
      const arm = new THREE.Group(); arm.rotation.z = i * Math.PI / 2;
      box(arm, '#e4d8ae', .12, .65, 0, .22, 1.05, .035); box(arm, palette.wood, 0, .6, 0, .045, 1.2, .06); windmill.add(arm);
    }
    g.add(windmill);
  } else if (type === 'monastery') {
    box(g, palette.limestone, -.8, 1.5, .4, .6, 2.8, .65); roof(g, -.8, 3, .4, .85, .6, .85);
    box(g, '#b7a060', -.8, 3.55, .4, .045, .45, .045); box(g, '#b7a060', -.8, 3.6, .4, .25, .045, .045);
  } else if(type === 'university') {
    for(const x of [-r*.6,-r*.2,r*.2,r*.6])shape(g,cylinder,palette.limestone,x,.75,r*.92,.09,1.4,.09);
    roof(g,0,1.8,r*.82,r*1.6,.45,.85);
    for(const side of [-1,1]){const page=box(g,'#e2d6ac',side*.18,2.28,0,.35,.06,.45);page.rotation.z=side*.16;}
    box(g,palette.wood,0,2.25,0,.06,.07,.46);
  } else if (type === 'house') {
    box(g,'#8e8771',r*.43,height+.59,-r*.25,.23,.8,.28);
    box(g,'#655e4e',r*.43,height+1.01,-r*.25,.3,.1,.34);
  } else if (type === 'blacksmith') {
    box(g, '#777865', .6, 1.6, .5, .35, 1.8, .4);
    box(g, '#e19953', .4, .3, -1, .35, .2, .25);
  } else if (type === 'market') {
    for (let i = 0; i < 3; i++) {
      box(g, ['#a86b4d', '#c8b96d', '#608278'][i], i - 1, .85, -1.5, .85, .08, .8);
      box(g, palette.wood, i - 1, .3, -1.5, .75, .5, .6);
    }
  } else {
    if (type === 'barracks') {
      for (const x of [-.6,.6]) box(g,palette.wood,x,.55,r*.91,.085,1.1,.085);
      box(g,palette.wood,0,1.05,r*.91,1.3,.085,.085);
      for (const x of [-.35,0,.35]) {
        const spear = box(g,'#898a76',x,.78,r*.96,.045,1.2,.045); spear.rotation.z = -.16;
      }
    }
    if(type === 'archery_range') {
      for(const x of [-.62,.62]){
        box(g,palette.wood,x,.5,r*.94,.07,1,.07);
        for(const [radius,color] of [[.34,'#c6b187'],[.23,'#8e5844'],[.11,'#ded2ae']] as const){
          const target=shape(g,cylinder,color,x,.87,r*.96+.04*(.35-radius),radius,.025,radius);target.rotation.x=Math.PI/2;
        }
      }
    } else if(type === 'stable') {
      box(g,palette.wood,0,.4,r*.96,1.75,.1,.1);
      for(const x of [-.7,0,.7])box(g,palette.wood,x,.53,r*.96,.07,1.06,.07);
      shape(g,orb,'#957049',.4,.72,r*.79,.18,.35,.2);shape(g,orb,'#957049',.4,.98,r*.93,.13,.18,.22);
      box(g,'#cab37b',-.45,.3,r*.82,.6,.45,.5);
    } else if(type === 'siege_workshop') {
      box(g,palette.wood,0,.43,r*.92,1.2,.12,.55);
      for(const x of [-.6,.6]){const wheel=shape(g,cylinder,'#4d4636',x,.29,r*.92,.27,.1,.27);wheel.rotation.z=Math.PI/2;}
      const arm=box(g,palette.wood,0,.9,r*.87,.12,1.3,.12);arm.rotation.x=-.4;
      box(g,palette.wood,0,1.45,r*.6,.45,.12,.35);
    }
    flag(g, e.owner, -r * .8, 1.1, -r * .6, .65);
  }
}
function unit(g: THREE.Group, e: EntityView) {
  const color = ownerColor(e.owner), type = e.type;
  if (['galley', 'fishing_ship', 'fire_ship', 'transport', 'trade_ship'].includes(type)) {
    box(g, '#71543e', 0, .2, 0, .65, .35, 1.8);
    const bow = shape(g, cone, '#71543e', 0, .2, -.9, .46, .5, .46); bow.rotation.x = Math.PI / 2;
    box(g, palette.wood, 0, .9, 0, .04, 1.5, .04);
    const sail=box(g, type === 'fire_ship' ? color : '#d9cfac', .27, 1.1, 0, .55, .8, .025);
    if((e.damage_stage??0)>=3){sail.scale.y*=.6;sail.rotation.z=.24}
    if(type==='trade_ship'){for(const z of [-.55,.5])box(g,'#b5a47c',0,.48,z,.48,.24,.38);flag(g,e.owner,-.24,.35,.7,.3)}
    return;
  }
  if (['ram', 'mangonel', 'trebuchet', 'bombard_cannon', 'trade_cart', 'supply_cart'].includes(type)) {
    box(g, palette.wood, 0, .35, 0, .8, .35, 1.2);
    for (const x of [-.45, .45]) for (const z of [-.4, .4]) { const wheel = shape(g, cylinder, '#443c2a', x, .23, z, .23, .12, .23); wheel.rotation.z = Math.PI / 2; }
    if (type === 'trebuchet') { box(g, palette.wood, 0, 1, 0, .12, 1.3, .12); const arm = box(g, '#9c7c4d', 0, 1.2, 0, .1, .1, 2); arm.rotation.x = e.deployed ? -.6 : 0; }
    else if (type === 'ram') roof(g, 0, .8, 0, 1, .6, 1.6);
    else if (type === 'supply_cart' || type === 'trade_cart') {
      box(g, '#9c7c4d', -.18, .64, -.22, .3, .3, .4);
      box(g, '#b5a47c', .17, .61, .2, .35, .26, .4);
      if (type === 'supply_cart') { box(g, '#d9cfac', 0, .95, 0, .85, .12, 1.2); for (const x of [-.36,.36]) box(g, palette.wood, x, .68, 0, .05, .5, .05); }
    }
    else { const arm = box(g, type === 'bombard_cannon' ? '#414b46' : palette.wood, 0, .75, -.2, .2, .2, 1.1); arm.rotation.x = -.3; }
    flag(g, e.owner, .4, .5, .4, .35); return;
  }
  const mounted = ['scout', 'knight', 'camel', 'cavalry_archer', 'cataphract', 'war_elephant', 'mameluke', 'mangudai'].includes(type);
  const baseY = mounted ? .55 : 0;
  if (mounted) {
    box(g, type === 'war_elephant' ? '#888b7c' : '#735a45', 0, .48, 0, .36, .42, .73);
    box(g, '#846c51', 0, .72, -.37, .25, .38, .24);
    for (const x of [-.13, .13]) for (const z of [-.23, .23]) box(g, '#554737', x, .18, z, .07, .35, .08);
  }
  if (type === 'monk') shape(g, pine, '#d6cfb4', 0, .32, 0, .27, .65, .27);
  else box(g, color, 0, .38 + baseY, 0, .29, .36, .21);
  shape(g, orb, '#cfb391', 0, .68 + baseY, 0, .135, .14, .13);
  if (!['villager', 'monk'].includes(type)) shape(g, orb, palette.metal, 0, .77 + baseY, 0, .15, .08, .15);
  for (const x of [-.09, .09]) { const leg = box(g, '#514b38', x, .14 + baseY, 0, .085, .26, .1); leg.name = 'leg'; }
  const tool = box(g, ['villager', 'monk'].includes(type) ? '#846945' : palette.metal, .24, .46 + baseY, 0, .035, type === 'spearman' ? 1.1 : .6, .045);
  tool.rotation.z = -.2; tool.name = 'tool';
  if (e.relic) shape(g, orb, '#e0bf63', -.23, .5, 0, .12, .17, .12);
}

export function makeModel(e: EntityView, biome = 'temperate') {
  const g = new THREE.Group(); g.userData.entityId = e.id;
  if (e.kind === 'building') building(g, e);
  else if (e.kind === 'unit') unit(g, e);
  else if (e.type === 'tree') {
    const variation = 1 + (e.id % 7) * .045;
    shape(g, cylinder, palette.wood, 0, .7, 0, .12, 1.4, .12);
    if (biome === 'desert' || biome === 'tropical') {
      if (biome === 'tropical') {
        for (let i = 0; i < 3; i++) shape(g, stone, ['#477a45','#5f8c49','#79a154'][i], Math.sin(i*2.4)*.4, 1.6+i*.2, Math.cos(i*2.4)*.3, .88*variation, .6, .85*variation);
      } else {
        for (let i = 0; i < 6; i++) {
          const a = i*Math.PI/3;
          const leaf = shape(g, orb, i%2 ? '#748547' : '#526f43', Math.cos(a)*.46, 1.52, Math.sin(a)*.46, .66, .11, .19);
          leaf.rotation.y = -a; leaf.rotation.z = -.22;
        }
      }
    } else if (biome === 'autumn') {
      for(let i=0;i<3;i++)shape(g,stone,['#986c3c','#b8863d','#a35432'][(e.id+i)%3],Math.sin(i*2.4)*.34,1.45+i*.24,Math.cos(i*2.4)*.3,.8*variation,.65*variation,.76*variation);
    } else if (biome === 'savanna') {
      for(let i=0;i<3;i++)shape(g,stone,['#7f8047','#999052','#6f7946'][i],(i-1)*.35,1.65+i*.13,Math.sin(i*2)*.22,.96*variation,.3,.78*variation);
    } else if (biome === 'alpine') {
      for (let i = 0; i < 3; i++) {
        shape(g, pine, '#42635b', 0, 1.1+i*.55, 0, (.83-i*.19)*variation, 1.4*variation, (.83-i*.19)*variation);
        shape(g, pine, '#d9e0d9', 0, 1.38+i*.55, 0, (.58-i*.13)*variation, .9*variation, (.58-i*.13)*variation);
      }
    } else if (e.id % 4 === 0) {
      for (let i = 0; i < 3; i++) shape(g,stone,['#71834d','#879457','#9aa160'][i],Math.sin(i*2.4)*.32,1.45+i*.25,Math.cos(i*2.4)*.27,.69*variation,.74*variation,.65*variation);
    } else for (let i = 0; i < 3; i++) shape(g, pine, ['#415d3e', '#536f43', '#688149'][(e.id + i) % 3], 0, 1.1 + i * .55, 0, (.83 - i * .19) * variation, 1.4 * variation, (.83 - i * .19) * variation);
  } else if (e.type === 'gold' || e.type === 'stone') {
    for (let i = 0; i < 3; i++) shape(g, stone, e.type === 'gold' ? ['#a49b67', '#c8b77d', '#dec37c'][i] : ['#a4a797', '#b7b8a7', '#92978a'][i], i * .35 - .35, .27 + (i % 2) * .14, (i % 2) * .3, .45, .43 + i * .08, .4);
  } else if (e.type === 'berries') {
    shape(g, orb, '#68794c', 0, .35, 0, .55, .45, .5);
    for (let i = 0; i < 5; i++) shape(g, orb, '#985556', Math.cos(i * 2) * .35, .55, Math.sin(i * 2) * .35, .06, .06, .06);
  } else if (e.type === 'sheep') {
    shape(g, orb, '#e5dfc8', 0, .25, 0, .25, .22, .37); shape(g, orb, '#6c6353', 0, .28, -.3, .12, .13, .14);
    for (const x of [-.12, .12]) for (const z of [-.17, .17]) box(g, '#6c6353', x, .07, z, .04, .16, .05);
  } else if (e.type === 'relic') {
    shape(g, cylinder, '#c2a154', 0, .2, 0, .15, .4, .15); shape(g, stone, '#ecd18d', 0, .55, 0, .2, .27, .2);
  } else if (e.type === 'fish') {
    // The water is opaque: surface silhouettes and ripples make shoals legible
    // while preserving terrain depth testing and normal canvas picking.
    for (let i = 0; i < 3; i++) {
      const x = (i - 1) * .25, z = (i % 2) * .29 - .14;
      shape(g, orb, '#d8e3cf', x, .065, z, .085, .035, .21);
      const tail = shape(g, cone, '#afc7b6', x, .065, z + .22, .115, .17, .035);
      tail.rotation.x = Math.PI / 2;
    }
    const ring = shape(g, ripple, '#9bbcac', 0, .03, 0, .64, .52, .64);
    ring.rotation.x = -Math.PI / 2;
  }
  if (e.progress < 1) {
    for (const x of [-e.radius, e.radius]) for (const z of [-e.radius, e.radius]) box(g, '#a8905b', x, 1, z, .07, 2, .07);
  }
  if (e.kind === 'building') dressBuilding(g, e);
  decorateDamage(g,e,roofGeometry);
  bakeStaticMeshes(g);
  if (!e.visible) {
    const faded = new Map<THREE.Material, THREE.MeshStandardMaterial>();
    g.traverse(o => {
      if (!(o instanceof THREE.Mesh)) return;
      const source = o.material as THREE.MeshStandardMaterial;
      let m = faded.get(source);
      if (!m) {
        // Damaged models already own their materials. Reuse those copies so
        // a remembered building does not abandon its first set of materials.
        m = o.userData.privateMaterial ? source : source.clone();
        m.color.multiplyScalar(.4); faded.set(source, m);
      }
      o.material = m; o.userData.privateMaterial = true;
    });
  }
  return g;
}
