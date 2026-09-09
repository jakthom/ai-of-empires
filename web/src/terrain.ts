import * as THREE from 'three';
import type { MapView, Vec } from './api.generated';
import { biomePalette } from './biomes';
import { changedMapCells } from './snapshot-stream';

// Relief is presentation of Go's observations, never a second terrain model
// for movement or combat. Unknown cells acquire heights only when Go reveals them.
export const terrainHeightScale = 4;
const chunkSize = 8, floor = -2.8;
const sides = [[0,1,-1,0,3,2],[1,2,0,1,0,3],[2,3,1,0,1,0],[3,0,0,-1,2,1]];
type Materials = { surface: THREE.MeshStandardMaterial[]; walls: THREE.MeshStandardMaterial };

function cornerHeights(map: MapView, i: number) {
  const tile = map.tiles[i], x = i % map.width, z = Math.floor(i / map.width);
  return [[x,z],[x,z+1],[x+1,z+1],[x+1,z]].map(([cx,cz]) => {
    let sum = 0, count = 0;
    for (const dx of [-1,0]) for (const dz of [-1,0]) {
      const nx = cx+dx, nz = cz+dz;
      const neighbor = nx >= 0 && nz >= 0 && nx < map.width && nz < map.height ? map.tiles[nz*map.width+nx] : undefined;
      if (neighbor?.terrain === tile.terrain) { sum += neighbor.elevation; count++; }
    }
    return (count ? sum/count : tile.elevation) * terrainHeightScale;
  });
}

class TerrainChunk {
  readonly group = new THREE.Group();
  readonly bounds = new THREE.Box3();
  private surface: THREE.Mesh<THREE.BufferGeometry, THREE.MeshStandardMaterial[]>;
  private walls: THREE.Mesh<THREE.BufferGeometry, THREE.MeshStandardMaterial>;
  private cells: number[] = [];
  private wallCells: number[] = [];

  constructor(map: MapView, corners: number[][], x0: number, z0: number, materials: Materials) {
    const positions: number[] = [], wallPositions: number[] = [], geometry = new THREE.BufferGeometry();
    const triangle = (points: number[][], cell: number, wall = false) => {
      (wall ? wallPositions : positions).push(...points.flat());
      (wall ? this.wallCells : this.cells).push(cell,cell,cell);
    };
    for (const water of [false,true]) {
      const start = positions.length / 3;
      for (let z = z0; z < Math.min(z0+chunkSize,map.height); z++) for (let x = x0; x < Math.min(x0+chunkSize,map.width); x++) {
        const i = z*map.width+x, tile = map.tiles[i];
        if (['water','shallows'].includes(tile.terrain) !== water) continue;
        const h = corners[i], p = [[x,h[0],z],[x,h[1],z+1],[x+1,h[2],z+1],[x+1,h[3],z]];
        triangle([p[0],p[1],p[2]],i); triangle([p[0],p[2],p[3]],i);
        for (const [a,b,dx,dz,na,nb] of sides) {
          const nx = x+dx, nz = z+dz;
          const n = nx >= 0 && nz >= 0 && nx < map.width && nz < map.height ? nz*map.width+nx : -1;
          const lowA = n < 0 ? floor : corners[n][na], lowB = n < 0 ? floor : corners[n][nb];
          if (h[a] <= lowA+.001 && h[b] <= lowB+.001) continue;
          const qa = [p[a][0],Math.min(h[a],lowA),p[a][2]], qb = [p[b][0],Math.min(h[b],lowB),p[b][2]];
          triangle([p[a],qa,qb],i,true); triangle([p[a],qb,p[b]],i,true);
        }
      }
      geometry.addGroup(start,positions.length/3-start,water ? 1 : 0);
    }
    const prepare = (geometry: THREE.BufferGeometry, positions: number[]) => {
      geometry.setAttribute('position',new THREE.Float32BufferAttribute(positions,3));
      geometry.setAttribute('color',new THREE.Float32BufferAttribute(new Float32Array(positions.length),3));
      geometry.computeVertexNormals(); geometry.computeBoundingBox(); geometry.computeBoundingSphere();
      if (positions.length) this.bounds.union(geometry.boundingBox!);
    };
    prepare(geometry,positions);
    this.surface = new THREE.Mesh(geometry,materials.surface); this.surface.receiveShadow = true;
    const walls = new THREE.BufferGeometry(); prepare(walls,wallPositions);
    this.walls = new THREE.Mesh(walls,materials.walls); this.walls.castShadow = true; this.walls.receiveShadow = true;
    this.group.add(this.surface,this.walls); this.updateFog(map);
  }

  updateFog(map: MapView) {
    const tint = new THREE.Color();
    for (const [mesh,cells,wall] of [[this.surface,this.cells,false],[this.walls,this.wallCells,true]] as const) {
      const attribute = mesh.geometry.getAttribute('color');
      cells.forEach((cell,vertex) => {
        const tile = map.tiles[cell], fog = map.fog[cell], colors = biomePalette(tile.biome || map.biome);
        tint.set(!fog ? '#19281f' : wall ? tile.terrain === 'cliff' ? colors.cliff : colors.earth : colors[tile.terrain] ?? colors.grass);
        if (fog) tint.multiplyScalar((.98+Math.sin(cell*7.3)*.025) * (fog === 1 ? .38 : 1));
        attribute.setXYZ(vertex,tint.r,tint.g,tint.b);
      });
      attribute.needsUpdate = true;
    }
  }

  hit(raycaster: THREE.Raycaster) {
    if (!raycaster.ray.intersectsBox(this.bounds)) return null;
    return raycaster.intersectObjects(this.group.children)[0]?.point ?? null;
  }
  dispose() { this.surface.geometry.dispose(); this.walls.geometry.dispose(); }
}

export class BattlefieldTerrain {
  readonly group = new THREE.Group();
  revision = 0;
  private corners: number[][];
  private chunks = new Map<number,TerrainChunk>();
  private columns: number;
  private materials: Materials = {
    surface: [
      new THREE.MeshStandardMaterial({ vertexColors: true, roughness: .98, flatShading: true }),
      new THREE.MeshStandardMaterial({ vertexColors: true, roughness: .32, metalness: .16, flatShading: true }),
    ],
    walls: new THREE.MeshStandardMaterial({ vertexColors: true, roughness: 1, side: THREE.DoubleSide }),
  };

  constructor(private map: MapView) {
    this.columns = Math.ceil(map.width/chunkSize);
    this.corners = map.tiles.map((_,i) => cornerHeights(map,i));
    for (let z = 0; z < map.height; z += chunkSize) for (let x = 0; x < map.width; x += chunkSize) this.rebuild(this.chunkAt(x,z));
  }
  private chunkAt(x: number,z: number) { return Math.floor(x/chunkSize)+Math.floor(z/chunkSize)*this.columns; }
  private rebuild(id: number) {
    const previous = this.chunks.get(id);
    if (previous) { this.group.remove(previous.group); previous.dispose(); }
    const chunk = new TerrainChunk(this.map,this.corners,id % this.columns*chunkSize,Math.floor(id/this.columns)*chunkSize,this.materials);
    this.chunks.set(id,chunk); this.group.add(chunk.group);
  }

  update(map: MapView) {
    if (map === this.map) return;
    const indices = changedMapCells(map, this.map);
    const changed = new Set<number>(), geometry = new Set<number>(), fog = new Set<number>();
    const inspect = (i: number) => {
      const tile = map.tiles[i];
      const old = this.map.tiles[i], x = i % map.width, z = Math.floor(i/map.width);
      if (tile.terrain !== old.terrain || tile.elevation !== old.elevation) {
        // A newly revealed neighbor changes joined corners and bordering walls.
        for (let dz = -2; dz <= 2; dz++) for (let dx = -2; dx <= 2; dx++) {
          const nx=x+dx,nz=z+dz;
          if (nx < 0 || nz < 0 || nx >= map.width || nz >= map.height) continue;
          changed.add(nz*map.width+nx); geometry.add(this.chunkAt(nx,nz));
        }
      }
      if (tile.biome !== old.biome || map.biome !== this.map.biome || map.fog[i] !== this.map.fog[i]) fog.add(this.chunkAt(x,z));
    };
    if (indices && map.biome === this.map.biome) indices.forEach(inspect);
    else for (let i = 0; i < map.tiles.length; i++) inspect(i);
    this.map = map;
    changed.forEach(i => { this.corners[i] = cornerHeights(map,i); });
    geometry.forEach(id => this.rebuild(id));
    if (geometry.size) this.revision++;
    fog.forEach(id => { if (!geometry.has(id)) this.chunks.get(id)?.updateFog(map); });
  }

  height(position: Vec) {
    const x=Math.floor(position.x),z=Math.floor(position.y);
    if (x < 0 || z < 0 || x >= this.map.width || z >= this.map.height) return 0;
    const h=this.corners[z*this.map.width+x],u=position.x-x,v=position.y-z;
    return v >= u ? h[0]*(1-v)+h[1]*(v-u)+h[2]*u : h[0]*(1-u)+h[2]*v+h[3]*(u-v);
  }

  hit(raycaster: THREE.Raycaster) {
    let nearest: THREE.Vector3 | null = null, distance = Infinity;
    for (const chunk of this.chunks.values()) {
      const hit = chunk.hit(raycaster);
      if (!hit) continue;
      const d=raycaster.ray.origin.distanceToSquared(hit);
      if (d < distance) { distance=d; nearest=hit; }
    }
    return nearest;
  }
  dispose() {
    this.chunks.forEach(chunk => chunk.dispose()); this.chunks.clear(); this.group.clear();
    this.materials.surface.forEach(material => material.dispose()); this.materials.walls.dispose();
  }
}
