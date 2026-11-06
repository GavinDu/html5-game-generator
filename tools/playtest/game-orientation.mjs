// Game orientation is an authored capability, never an inference from the device.
export function gameOrientation(ir) {
  const value = ir?.game?.orientation;
  if (!['portrait', 'landscape', 'any'].includes(value)) throw new Error('Unsupported game orientation: ' + value);
  return value;
}
export function supportsViewport(orientation, width, height) {
  return orientation === 'any' || (orientation === 'landscape' ? width > height : height >= width);
}
export function preferredViewport(ir, shortEdge = 430, longEdge = 960) {
  const orientation = gameOrientation(ir);
  const scene = ir.scenes?.find(s => s.id === ir.game.entryScene) || ir.scenes?.[0];
  const wide = orientation === 'landscape' || (orientation === 'any' && scene?.worldSize?.[0] > scene?.worldSize?.[1]);
  return wide ? { width: longEdge, height: shortEdge } : { width: shortEdge, height: longEdge };
}
