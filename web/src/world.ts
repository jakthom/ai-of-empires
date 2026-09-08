import * as THREE from 'three';
import type { EntityView, Snapshot, Vec } from './api.generated';
import { makeModel, ownerColor } from './models';
import { biomePalette } from './biomes';
import { BattlefieldTerrain } from './terrain';
import { terrainAnchorOffset } from './camera';
import { BuildingFoundation, GroundRing, groundFarm } from './grounding';

type RenderEntity = { object: THREE.Group; foundation?: BuildingFoundation; animated: THREE.Object3D[]; from: THREE.Vector3; to: THREE.Vector3; at: number; signature: string; view: EntityView };
// THESIS: a miniature landscape with real perspective and readable relief.
// OWN-WORLD: limestone, timber, terracotta, sage terrain, and soft daylight.
// STORY: hold to grab the land; hold both mouse buttons to inspect its volume.
// FIRST VIEWPORT: the settlement leads above the existing RTS command console.
// FORM: extend the incumbent battlefield; all geometry projects Go observations.
const defaultView = { zoom: 13, yaw: Math.PI / 4, tilt: Math.PI * .24, fov: 38 };

export class WorldRenderer {
  readonly renderer: THREE.WebGLRenderer;
  readonly camera = new THREE.PerspectiveCamera(defaultView.fov, 1, .1, 350);
  readonly scene = new THREE.Scene();
  readonly target = new THREE.Vector3(19, 0, 42);
  readonly entities = new Map<number, RenderEntity>();
  readonly keys = new Set<string>();
  readonly selected = new Set<number>();
  readonly canvas: HTMLCanvasElement;
  private terrain?: BattlefieldTerrain;
  private snapshot?: Snapshot;
  private rings = new THREE.Group();
  private projectiles = new THREE.Group();
  private projectileMeshes = new Map<number, THREE.Mesh>();
  private projectileGeometry = { stone: new THREE.SphereGeometry(.12, 5, 4), arrow: new THREE.BoxGeometry(.035, .035, .4) };
  private projectileMaterial = { stone: new THREE.MeshBasicMaterial({ color: '#c2baa3' }), arrow: new THREE.MeshBasicMaterial({ color: '#604323' }) };
  private hovered: number | null = null;
  private raycaster = new THREE.Raycaster();
  private plane = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
  private ghost?: THREE.Group;
  private marker?: GroundRing;
  private markerPoint?: Vec;
  private markerAt = 0;
  private zoom = defaultView.zoom;
  private yaw = defaultView.yaw;
  private tilt = defaultView.tilt;
  private lastFrame = performance.now();
  private resizeObserver: ResizeObserver;
  private animation = 0;
  private running = true;
  private reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
  private sun = new THREE.DirectionalLight('#fff0d3', 3.2);

  constructor(private container: HTMLElement) {
    this.renderer = new THREE.WebGLRenderer({ antialias: true, alpha: false, powerPreference: 'high-performance' });
    this.renderer.setPixelRatio(Math.min(devicePixelRatio, 1.75));
    this.renderer.shadowMap.enabled = true; this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping; this.renderer.toneMappingExposure = 1.12;
    this.renderer.setClearColor('#82988a'); this.scene.fog = new THREE.Fog('#82988a', 65, 160); this.canvas = this.renderer.domElement;
    const backdrop = new THREE.Mesh(new THREE.PlaneGeometry(900, 900), new THREE.MeshBasicMaterial({ color: '#19281f' }));
    backdrop.rotation.x = -Math.PI / 2; backdrop.position.set(36, -2.85, 36); this.scene.add(backdrop);
    this.canvas.setAttribute('aria-label', 'Interactive battlefield. Click to select. Hold the left button briefly, then drag to pan. Hold both left and right mouse buttons and drag to rotate and tilt, or use Shift and arrow keys. Shift-drag selects a group. Choose Give order or press Q, then click to command.');
    this.canvas.tabIndex = 0; container.append(this.canvas);
    this.scene.add(new THREE.HemisphereLight('#d9e9ec', '#635b40', 1.6));
    const sun = this.sun; sun.castShadow = true;
    sun.shadow.mapSize.set(2048, 2048); sun.shadow.camera.left = -38; sun.shadow.camera.right = 38;
    sun.shadow.camera.top = 38; sun.shadow.camera.bottom = -38; sun.shadow.camera.far = 130;
    sun.shadow.bias = -.0002; sun.shadow.normalBias = .035; this.scene.add(sun, sun.target);
    this.scene.add(this.rings, this.projectiles);
    this.resizeObserver = new ResizeObserver(() => this.resize()); this.resizeObserver.observe(container); this.resize();
    this.canvas.addEventListener('wheel', e => {
      e.preventDefault();
      const unit = e.deltaMode === WheelEvent.DOM_DELTA_LINE ? 16 : e.deltaMode === WheelEvent.DOM_DELTA_PAGE ? this.container.clientHeight : 1;
      this.zoomBy(e.deltaY * unit * .012, { x: e.clientX, y: e.clientY });
    }, { passive: false });
    this.canvas.addEventListener('webglcontextlost', e => { e.preventDefault(); this.running = false; container.dispatchEvent(new CustomEvent('renderlost')); });
    this.canvas.addEventListener('webglcontextrestored', () => { this.running = true; if (this.snapshot) this.update(this.snapshot); container.dispatchEvent(new CustomEvent('renderrestored')); });
    this.frame();
  }
  private resize() {
    const w = this.container.clientWidth, h = this.container.clientHeight;
    this.renderer.setSize(w, h); const aspect = w / Math.max(1, h);
    this.camera.aspect = aspect;
    this.camera.updateProjectionMatrix(); this.positionCamera();
  }
  private positionCamera() {
    const distance = this.zoom / Math.tan(THREE.MathUtils.degToRad(this.camera.fov / 2));
    const horizontal = distance * Math.cos(this.tilt);
    this.camera.position.copy(this.target).add(new THREE.Vector3(horizontal * Math.sin(this.yaw), distance * Math.sin(this.tilt), horizontal * Math.cos(this.yaw)));
    this.camera.lookAt(this.target); this.camera.updateMatrixWorld();
    // Keep a detailed, stable shadow volume around the part of the map in view.
    const x = Math.round(this.target.x / 2) * 2, z = Math.round(this.target.z / 2) * 2;
    this.sun.target.position.set(x, 0, z); this.sun.position.set(x - 28, 48, z - 18);
    this.container.dispatchEvent(new Event('viewchange'));
  }
  focus(position: Vec) { this.target.set(position.x, this.elevation(position), position.y); this.positionCamera(); }
  zoomBy(delta: number, cursor?: { x: number; y: number }) {
    const zoom = THREE.MathUtils.clamp(this.zoom + delta, 5, 30);
    if (zoom === this.zoom) return;
    const anchor = cursor ? this.groundHit(cursor.x, cursor.y) : null;
    this.zoom = zoom; this.resize();
    if (anchor && cursor) {
      // Intersect the cursor ray at the grabbed point's height, then translate
      // horizontally. Keeping the pivot height fixed avoids accumulating
      // vertical drift when zooming alternately over hills and low ground.
      this.setRay(cursor.x, cursor.y);
      const offset = terrainAnchorOffset(this.raycaster.ray, anchor);
      if (offset) { this.target.add(offset); this.positionCamera(); }
    }
  }
  orbitBy(dx: number, dy: number) {
    this.yaw = (this.yaw - dx * .006) % (Math.PI * 2);
    this.tilt = THREE.MathUtils.clamp(this.tilt + dy * .004, Math.PI * .14, Math.PI * .44);
    this.positionCamera();
  }
  resetView() { this.yaw = defaultView.yaw; this.tilt = defaultView.tilt; this.zoom = defaultView.zoom; this.resize(); }
  pan(dx: number, dy: number) {
    const r = this.canvas.getBoundingClientRect(), x = r.left + r.width / 2, y = r.top + r.height / 2;
    this.panBetween(x, y, x - dx, y - dy);
  }
  panBetween(fromX: number, fromY: number, toX: number, toY: number) {
    const anchor = this.groundHit(fromX, fromY);
    if (!anchor) return;
    this.setRay(toX, toY);
    const offset = terrainAnchorOffset(this.raycaster.ray, anchor);
    if (!offset) return;
    this.target.add(offset);
    this.clampCamera();
  }
  private clampCamera() { this.target.x = THREE.MathUtils.clamp(this.target.x, 1, (this.snapshot?.map.width ?? 72) - 2); this.target.z = THREE.MathUtils.clamp(this.target.z, 1, (this.snapshot?.map.height ?? 72) - 2); this.positionCamera(); }
  private elevation(position: Vec) { return this.terrain?.height(position) ?? 0; }

  update(snapshot: Snapshot) {
    const previous = this.snapshot;
    this.snapshot = snapshot;
    const map = snapshot.map;
    if (previous?.map.biome !== map.biome) {
      const palette = biomePalette(map.biome);
      this.renderer.setClearColor(palette.sky); this.scene.fog = new THREE.Fog(palette.sky, 65, 160);
    }
    if (this.terrain && previous && (previous.map.width !== map.width || previous.map.height !== map.height)) {
      this.scene.remove(this.terrain.group); this.terrain.dispose(); this.terrain = undefined;
    }
    if (!this.terrain) { this.terrain = new BattlefieldTerrain(map); this.scene.add(this.terrain.group); }
    else this.terrain.update(map);
    const alive = new Set<number>(), now = performance.now();
    for (const e of snapshot.entities) {
      if (e.container) continue;
      alive.add(e.id);
      const biome = map.tiles[Math.floor(e.position.y)*map.width+Math.floor(e.position.x)]?.biome || map.biome;
      const signature = `${biome}:${e.appearance_age ?? 0}:${e.type}:${e.owner}:${e.visible}:${e.progress < 1}:${e.deployed}:${e.relic}:${e.type === 'farm' && (e.amount ?? 0) <= 0}`;
      let rendered = this.entities.get(e.id);
      if (!rendered || rendered.signature !== signature) {
        if (rendered) this.removeModel(rendered.object);
        const object = makeModel(e, biome); this.scene.add(object);
        if (e.type === 'farm') groundFarm(object, e.position, this.terrain);
        const foundation = e.kind === 'building' && !['farm', 'dock'].includes(e.type) ? new BuildingFoundation(e) : undefined;
        if (foundation) object.add(foundation);
        const pos = new THREE.Vector3(e.position.x, this.elevation(e.position), e.position.y);
        const animated: THREE.Object3D[] = [];
        object.traverse(o => { if (['leg', 'tool', 'windmill', 'flag'].includes(o.name)) animated.push(o); });
        rendered = { object, foundation, animated, from: pos.clone(), to: pos.clone(), at: now, signature, view: e }; this.entities.set(e.id, rendered);
      }
      if (e.kind === 'building') rendered.object.scale.y = e.type === 'farm' ? 1 : .2 + .8 * e.progress;
      const height = rendered.foundation?.fit(e.position, this.terrain, rendered.object.scale.y) ?? this.elevation(e.position);
      rendered.from.copy(rendered.object.position); rendered.to.set(e.position.x, height, e.position.y);
      if (rendered.at === now) rendered.from.copy(rendered.to);
      rendered.at = now; rendered.view = e;
    }
    for (const [id, e] of this.entities) if (!alive.has(id)) { this.removeModel(e.object); this.entities.delete(id); this.selected.delete(id); }
    this.updateRings();
    const projectiles = new Set(snapshot.projectiles.map(p => p.id));
    for (const [id, mesh] of this.projectileMeshes) if (!projectiles.has(id)) { this.projectiles.remove(mesh); this.projectileMeshes.delete(id); }
    const previousProjectiles = new Map(previous?.projectiles.map(p => [p.id, p]));
    for (const p of snapshot.projectiles) {
      let mesh = this.projectileMeshes.get(p.id);
      if (!mesh) { const kind = p.kind === 'stone' ? 'stone' : 'arrow'; mesh = new THREE.Mesh(this.projectileGeometry[kind], this.projectileMaterial[kind]); this.projectileMeshes.set(p.id, mesh); this.projectiles.add(mesh); }
      mesh.position.set(p.position.x, this.elevation(p.position) + 1.1, p.position.y);
      const last = previousProjectiles.get(p.id);
      if (last) mesh.lookAt(2 * p.position.x - last.position.x, mesh.position.y, 2 * p.position.y - last.position.y);
    }
  }
  private removeModel(object: THREE.Group) {
    this.scene.remove(object);
    object.traverse(o => { if (o instanceof THREE.Mesh && o.userData.privateMaterial) (o.material as THREE.Material).dispose(); if (o instanceof THREE.Mesh && o.userData.privateGeometry) o.geometry.dispose(); if (o instanceof THREE.InstancedMesh) o.dispose(); });
  }
  private updateRings() {
    const wanted = new Set(this.selected); if (this.hovered !== null) wanted.add(this.hovered);
    for (const o of [...this.rings.children]) if (!wanted.has(o.userData.forEntity) || !this.entities.has(o.userData.forEntity)) {
      (o as GroundRing).dispose(); this.rings.remove(o);
    }
    for (const id of wanted) {
      const e = this.entities.get(id); if (!e) continue;
      const radius = Math.max(.5, e.view.radius + .12);
      let ring = this.rings.children.find(r => r.userData.forEntity === id) as GroundRing | undefined;
      if (!ring) { ring = new GroundRing(); ring.userData.forEntity = id; this.rings.add(ring); }
      ring.place({ x: e.object.position.x, y: e.object.position.z }, radius, this.terrain);
      ring.material.color.set(this.hovered === id && !this.selected.has(id) ? '#e3c984' : ownerColor(e.view.owner));
    }
  }
  setSelection(ids: number[]) { this.selected.clear(); ids.forEach(id => this.selected.add(id)); this.updateRings(); }
  setHovered(id: number | null) { if (this.hovered === id) return; this.hovered = id; this.updateRings(); }
  resetWorld() {
    this.clearMarker();
    this.preview(null); this.setHovered(null); this.selected.clear();
    for (const e of this.entities.values()) this.removeModel(e.object);
    this.entities.clear(); this.updateRings(); this.projectiles.clear(); this.projectileMeshes.clear();
    if (this.terrain) { this.scene.remove(this.terrain.group); this.terrain.dispose(); this.terrain = undefined; }
    this.snapshot = undefined; this.target.set(19, 0, 42); this.resetView();
  }
  private setRay(clientX: number, clientY: number) {
    const rect = this.canvas.getBoundingClientRect(); this.raycaster.setFromCamera(new THREE.Vector2((clientX - rect.left) / rect.width * 2 - 1, -(clientY - rect.top) / rect.height * 2 + 1), this.camera);
  }
  private groundHit(clientX: number, clientY: number) {
    this.setRay(clientX, clientY);
    const terrainHit = this.terrain?.hit(this.raycaster);
    return terrainHit ?? this.raycaster.ray.intersectPlane(this.plane, new THREE.Vector3());
  }
  groundPoint(clientX: number, clientY: number): Vec | null {
    const hit = this.groundHit(clientX, clientY); return hit ? { x: hit.x, y: hit.z } : null;
  }
  pick(clientX: number, clientY: number): number | null {
    this.setRay(clientX, clientY);
    const hits = this.raycaster.intersectObjects([...this.entities.values()].map(e => e.object), true);
    if (!hits.length) return null;
    const terrainHit = this.terrain?.hit(this.raycaster);
    if (terrainHit && terrainHit.distanceTo(this.raycaster.ray.origin) < hits[0].distance - .02) return null;
    let object: THREE.Object3D | null = hits[0].object;
    while (object && !object.userData.entityId) object = object.parent;
    return object?.userData.entityId ?? null;
  }
  screenPoint(id: number) {
    const e = this.entities.get(id); if (!e) return null;
    const p = e.object.position.clone().project(this.camera), r = this.canvas.getBoundingClientRect();
    return { x: (p.x + 1) / 2 * r.width + r.left, y: (-p.y + 1) / 2 * r.height + r.top };
  }
  preview(entity: EntityView | null, point?: Vec, valid?: boolean) {
    if (this.ghost) { this.removeModel(this.ghost); this.ghost = undefined; }
    if (!entity || !point) return;
    this.ghost = makeModel(entity);
    const center = { x: Math.floor(point.x) + .5, y: Math.floor(point.y) + .5 };
    let height = this.elevation(center);
    if (this.terrain) {
      if (entity.type === 'farm') groundFarm(this.ghost, center, this.terrain);
      else if (entity.kind === 'building' && entity.type !== 'dock') {
        const foundation = new BuildingFoundation(entity); this.ghost.add(foundation);
        height = foundation.fit(center, this.terrain);
      }
    }
    this.ghost.position.set(center.x, height, center.y);
    this.ghost.traverse(o => { if (o instanceof THREE.Mesh) { if (o.userData.privateMaterial) (o.material as THREE.Material).dispose(); o.material = new THREE.MeshBasicMaterial({ color: valid === false ? '#d77f66' : valid === true ? '#c2d6a0' : '#d5c79e', transparent: true, opacity: .5 }); o.userData.privateMaterial = true; } });
    this.scene.add(this.ghost);
  }
  orderMarker(point: Vec) {
    this.clearMarker();
    this.marker = new GroundRing(.43 / .35, true); this.marker.material.color.set('#fff2c0');
    this.markerPoint = point; this.marker.place(point, .35, this.terrain); this.markerAt = performance.now(); this.scene.add(this.marker);
  }
  private clearMarker() { if (this.marker) { this.scene.remove(this.marker); this.marker.dispose(); this.marker = undefined; this.markerPoint = undefined; } }
  private frame = () => {
    this.animation = requestAnimationFrame(this.frame);
    const now = performance.now(), dt = Math.min(.05, (now - this.lastFrame) / 1000); this.lastFrame = now;
    let dx = 0, dy = 0;
    if (this.keys.has('ArrowLeft')) dx -= 1; if (this.keys.has('ArrowRight')) dx += 1; if (this.keys.has('ArrowUp')) dy -= 1; if (this.keys.has('ArrowDown')) dy += 1;
    if (dx || dy) {
      if (this.keys.has('Shift')) this.orbitBy(dx * dt * 150, dy * dt * 150);
      else this.pan(dx * dt * 480, dy * dt * 480);
    }
    for (const e of this.entities.values()) {
      const t = THREE.MathUtils.clamp((now - e.at) / 100, 0, 1);
      e.object.position.lerpVectors(e.from, e.to, t);
      e.object.position.y = e.view.kind === 'building' ? e.to.y : this.elevation({ x: e.object.position.x, y: e.object.position.z });
      const moving = e.from.distanceToSquared(e.to) > .0001;
      if (moving && e.view.kind === 'unit') e.object.rotation.y = Math.atan2(e.to.x - e.from.x, e.to.z - e.from.z) + Math.PI;
      e.animated.forEach(o => {
        if (this.reducedMotion.matches || this.snapshot?.paused) { if (o.name === 'leg' || o.name === 'tool') o.rotation.x = 0; return; }
        if (o.name === 'windmill') o.rotation.z = now * .00025;
        if (o.name === 'flag') o.rotation.y = Math.sin(now * .002 + e.view.id) * .1;
        if (o.name === 'leg') o.rotation.x = moving ? Math.sin(now * .012 + o.position.x * 10) * .35 : 0;
        if (o.name === 'tool') o.rotation.x = ['gathering', 'constructing', 'repairing', 'attacking'].includes(e.view.state) ? Math.sin(now * .008) * .4 : 0;
      });
    }
    this.rings.children.forEach(r => { const e = this.entities.get(r.userData.forEntity); if (e) (r as GroundRing).place({ x: e.object.position.x, y: e.object.position.z }, Math.max(.5, e.view.radius + .12), this.terrain); });
    if (this.marker && this.markerPoint) {
      const age = (now - this.markerAt) / 800;
      if (age >= 1) this.clearMarker();
      else { this.marker.place(this.markerPoint, .35 * (1 + age), this.terrain); this.marker.material.opacity = 1 - age; }
    }
    if (this.running) this.renderer.render(this.scene, this.camera);
  };
  dispose() { cancelAnimationFrame(this.animation); this.resizeObserver.disconnect(); this.renderer.dispose(); }
}
