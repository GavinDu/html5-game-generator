import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { PNG } from 'pngjs';

// Source-color acceptance: questionnaire and the three submitted coloring pages
// require different *penguins*, not only different scarf IDs.
function sprite(variant) {
  const file = new URL('../../inventory/p3ngui/assets/penguin-' + variant + '.png.b64', import.meta.url);
  const image = PNG.sync.read(Buffer.from(readFileSync(file, 'utf8').trim(), 'base64'));
  assert.ok(image.width >= 112 && image.height >= 130, variant + ': source character too small');
  return image;
}
function pixel(image,x,y) {
  const i=(y*image.width+x)*4;
  return [image.data[i],image.data[i+1],image.data[i+2],image.data[i+3]];
}
function headColors(image) {
  // Restrict the sample to the crown, not the scarf, feet or scenery.
  const colored=[];
  for(let y=Math.floor(image.height*.05);y<Math.floor(image.height*.34);y++)
    for(let x=Math.floor(image.width*.20);x<Math.floor(image.width*.80);x++){
      const [r,g,b,a]=pixel(image,x,y);
      colored.push(a>180 && Math.max(r,g,b)>120 &&
        Math.max(r,g,b)-Math.min(r,g,b)>45);
    }
  return colored.filter(Boolean).length/colored.length;
}
function headDifference(a,b) {
  // The variants need visibly different head artwork at game resolution.
  let different=0,total=0;
  for(let yi=5;yi<34;yi++)
    for(let xi=20;xi<80;xi++){
      const x1=Math.floor(a.width*xi/100),y1=Math.floor(a.height*yi/100);
      const x2=Math.floor(b.width*xi/100),y2=Math.floor(b.height*yi/100);
      const [r1,g1,b1,a1]=pixel(a,x1,y1),[r2,g2,b2,a2]=pixel(b,x2,y2);
      if(a1>180&&a2>180){
        total++;
        if(Math.abs(r1-r2)+Math.abs(g1-g2)+Math.abs(b1-b2)>80) different++;
      }
    }
  return total ? different/total : 0;
}
test('rainbow and holiday have multicolored crowns, classic remains monochrome',()=>{
  const rainbow=sprite('rainbow'),holiday=sprite('holiday'),classic=sprite('classic');
  assert.ok(headColors(rainbow)>.15,'rainbow head needs source-inspired colored patches');
  assert.ok(headColors(holiday)>.15,'holiday head needs source-inspired colored sections');
  assert.ok(headColors(classic)<.18,'classic must retain its dark, source-drawn head');
  assert.ok(headDifference(rainbow,holiday)>.25,
    'rainbow and holiday must differ in head/body artwork beyond scarf');
});
test('sprites remain valid transparent PNGs with authored silhouette',()=>{
  for(const variant of ['rainbow','holiday','classic']){
    const image=sprite(variant);
    assert.ok(pixel(image,0,0)[3]<40,variant+' must not include photographed paper');
    assert.ok(pixel(image,Math.floor(image.width/2),Math.floor(image.height*.45))[3]>180,
      variant+' needs an opaque visible character face');
  }
});
