import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { inflateSync } from 'node:zlib';

// These source-authored sprites are 8-bit indexed PNGs with an alpha palette.
// Decode the actual pixel data using the PNG scanline filters instead of
// relying on pngjs's synchronous reader, which rejects this valid palette
// format on the CI Node runtime with "unrecognised content at end of stream".
function readIndexedPNG(data) {
  assert.equal(data.subarray(0, 8).toString('hex'), '89504e470d0a1a0a', 'PNG signature');
  let width = 0, height = 0, palette = null, alpha = null, ended = false;
  const compressed = [];
  for (let offset = 8; offset < data.length;) {
    assert.ok(offset + 12 <= data.length, 'truncated PNG chunk header');
    const length = data.readUInt32BE(offset);
    const end = offset + 12 + length;
    assert.ok(end <= data.length, 'truncated PNG chunk data');
    const type = data.toString('ascii', offset + 4, offset + 8);
    const chunk = data.subarray(offset + 8, offset + 8 + length);
    // Validate PNG CRCs, including the palette and transparency tables.
    let crc = 0xffffffff;
    for (let n = offset + 4; n < offset + 8 + length; n++) {
      crc ^= data[n];
      for (let k = 0; k < 8; k++)
        crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0);
    }
    assert.equal((crc ^ 0xffffffff) >>> 0, data.readUInt32BE(end - 4),
      type + ': invalid PNG CRC');
    if (type === 'IHDR') {
      assert.equal(length, 13);
      width = chunk.readUInt32BE(0);
      height = chunk.readUInt32BE(4);
      assert.equal(chunk[8], 8, 'PNG must be 8-bit');
      assert.equal(chunk[9], 3, 'PNG must be indexed/paletted');
      assert.equal(chunk[12], 0, 'interlaced PNG not supported by fidelity reader');
    } else if (type === 'PLTE') {
      assert.equal(length % 3, 0, 'malformed PNG palette');
      palette = chunk;
    } else if (type === 'tRNS') {
      alpha = chunk;
    } else if (type === 'IDAT') {
      compressed.push(chunk);
    } else if (type === 'IEND') {
      assert.equal(length, 0);
      assert.equal(end, data.length, 'unexpected trailing PNG content');
      ended = true;
    } else {
      assert.ok(type[0] === type[0].toLowerCase(), 'unknown critical PNG chunk ' + type);
    }
    offset = end;
  }
  assert.ok(ended && width > 0 && height > 0 && palette && compressed.length,
    'PNG missing required chunks');
  assert.ok(width * height <= 4096 * 4096, 'unreasonable sprite size');
  const rows = inflateSync(Buffer.concat(compressed));
  assert.equal(rows.length, (width + 1) * height, 'wrong uncompressed PNG scanline size');
  const pixels = Buffer.alloc(width * height * 4);
  let prior = Buffer.alloc(width);
  for (let y = 0; y < height; y++) {
    const filter = rows[y * (width + 1)];
    assert.ok(filter >= 0 && filter <= 4, 'invalid PNG scanline filter');
    const current = Buffer.alloc(width);
    for (let x = 0; x < width; x++) {
      const left = x ? current[x - 1] : 0;
      const up = prior[x];
      const upperLeft = x ? prior[x - 1] : 0;
      let prediction = 0;
      if (filter === 1) prediction = left;
      if (filter === 2) prediction = up;
      if (filter === 3) prediction = (left + up) >>> 1;
      if (filter === 4) {
        const p = left + up - upperLeft;
        const a = Math.abs(p - left), b = Math.abs(p - up), c = Math.abs(p - upperLeft);
        prediction = a <= b && a <= c ? left : b <= c ? up : upperLeft;
      }
      const index = (rows[y * (width + 1) + x + 1] + prediction) & 255;
      current[x] = index;
      assert.ok(index * 3 + 2 < palette.length, 'PNG pixel uses undefined palette entry');
      const dest = (y * width + x) * 4;
      pixels[dest] = palette[index * 3];
      pixels[dest + 1] = palette[index * 3 + 1];
      pixels[dest + 2] = palette[index * 3 + 2];
      pixels[dest + 3] = alpha?.[index] ?? 255;
    }
    prior = current;
  }
  return { width, height, data: pixels };
}

// Source-color acceptance: questionnaire and the three submitted coloring pages
// require different *penguins*, not only different scarf IDs.
function sprite(variant) {
  const file = new URL('../../inventory/p3ngui/assets/penguin-' + variant + '.png.b64', import.meta.url);
  const image = readIndexedPNG(Buffer.from(readFileSync(file, 'utf8').trim(), 'base64'));
  assert.equal(image.width, 160, variant + ': texture width must match classic source skin');
  assert.equal(image.height, 190, variant + ': texture height must match classic source skin');
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
