import type { Snapshot, Vec } from '../../src/api.generated';
import { test, expect, battlefieldKey, projectOpening } from './fixtures';
import { opening, build, select } from './economy-helpers';

// Choose a clear corridor from our authenticated map, then issue every order
// through public controls. The test never changes terrain or unit snapshots.
function corridor(v:Snapshot):[Vec,Vec]{
  const home=v.entities.find(e=>e.type==='town_center'&&e.owner===v.player.id)!.position;
  const obstacles=v.entities.filter(e=>e.kind!=='unit'&&!['farm','fish','sheep','berries','relic'].includes(e.type));
  for(let radius=12;radius<Math.max(v.map.width,v.map.height);radius+=3)for(let angle=0;angle<Math.PI*2;angle+=Math.PI/12)for(const turn of [-1,1]){
    const start={x:home.x+Math.cos(angle)*radius,y:home.y+Math.sin(angle)*radius};
    const direction={x:-Math.sin(angle)*turn,y:Math.cos(angle)*turn};
    const end={x:start.x+direction.x*18,y:start.y+direction.y*18};
    let clear=true;
    for(let step=-3;step<=21&&clear;step++)for(let side=-4;side<=4&&clear;side++){
      const p={x:start.x+direction.x*step-direction.y*side,y:start.y+direction.y*step+direction.x*side};
      const tile=v.map.tiles[Math.floor(p.y)*v.map.width+Math.floor(p.x)];
      if(p.x<2||p.y<2||p.x>v.map.width-2||p.y>v.map.height-2||!tile||!['grass','sand','snow'].includes(tile.terrain)||obstacles.some(e=>Math.hypot(e.position.x-p.x,e.position.y-p.y)<e.radius+.5))clear=false;
    }
    if(clear)return[start,end];
  }
  throw new Error('No observed clear marching corridor.');
}

test('marches a trained army in ranks and records normal-speed rendering',async({page,game},info)=>{
  test.setTimeout(150_000);
  await opening(page,game);
  for(let i=0;i<5;i++)await build(page,game,'house','House');
  await build(page,game,'barracks','Barracks');
  await select(page,game,'barracks');
  for(const total of [12,24]){
    for(let i=0;i<12;i++)await game.command('train',()=>page.locator('#actions').getByRole('button',{name:/^Militia/}).click());
    await expect.poll(async()=>(await game.snapshot()).entities.filter(e=>e.type==='militia'&&e.owner===1).length,{timeout:20_000}).toBe(total);
  }
  await battlefieldKey(page,'h');
  const box=(await page.locator('#world canvas').boundingBox())!;
  await page.keyboard.down('Shift');await page.mouse.move(box.x+8,box.y+8);await page.mouse.down();
  await page.mouse.move(box.x+box.width-8,box.y+box.height-8,{steps:8});await page.mouse.up();await page.keyboard.up('Shift');
  await battlefieldKey(page,'Control+2');
  const before=await game.snapshot(),[start,end]=corridor(before);
  const minimap=(await page.locator('#minimap').boundingBox())!;
  const center={x:(start.x+end.x)/2,y:(start.y+end.y)/2};
  await page.mouse.click(minimap.x+center.x/before.map.width*minimap.width,minimap.y+center.y/before.map.height*minimap.height);
  const staging=await projectOpening(page,before,start,0,center);
  const response=await game.command('move',()=>battlefieldKey(page,'q').then(()=>page.mouse.click(staging.x,staging.y)));
  const ids=response.request().postDataJSON().entity_ids as number[];
  expect(before.entities.filter(e=>ids.includes(e.id)&&e.type==='militia')).toHaveLength(24);
  await expect.poll(async()=>(await game.snapshot()).entities.filter(e=>ids.includes(e.id)).every(e=>e.state==='idle'),{timeout:30_000}).toBe(true);
  await game.command('speed',()=>page.locator('#speed').click());await expect(page.locator('#speed')).toHaveText('1×');
  const destination=await projectOpening(page,await game.snapshot(),end,0,center);
  await game.command('move',()=>battlefieldKey(page,'q').then(()=>page.mouse.click(destination.x,destination.y)));
  const frames=await page.evaluate(()=>new Promise<{p95Ms:number;over50:number}>(resolve=>{
    const times:number[]=[];const start=performance.now();let prior=start;
    function sample(now:number){times.push(now-prior);prior=now;if(now-start<8_000)requestAnimationFrame(sample);else{times.sort((a,b)=>a-b);resolve({p95Ms:times[Math.floor(times.length*.95)],over50:times.filter(x=>x>50).length})}}
    requestAnimationFrame(sample);
  }));
  const marching=(await game.snapshot()).entities.filter(e=>ids.includes(e.id));
  expect(marching.filter(e=>e.state==='moving').length).toBeGreaterThan(20);
  const dx=(end.x-start.x)/18,dy=(end.y-start.y)/18;
  const across=marching.map(e=>(e.position.y-start.y)*dx-(e.position.x-start.x)*dy);
  expect(Math.max(...across)-Math.min(...across)).toBeLessThan(8);
  await page.screenshot({path:info.outputPath('army-marching.png')});
  await info.attach('army-rendering.json',{body:JSON.stringify({units:marching.length,speed:1,frames},null,2),contentType:'application/json'});
  console.log('ARMY_RENDERING',JSON.stringify({units:marching.length,speed:1,frames}));
  expect(frames.p95Ms).toBeLessThan(50);
});
