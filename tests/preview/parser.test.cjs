// Pure parsing helpers are checked independently; DOM/XML behavior is exercised in real Chromium.
'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const parser = require('../../web/preview/parser.js');

// Saved-file URLs must stay in this generator instance and cannot escape to another endpoint or origin.
test('output reads stay on the exact same-origin saved XML endpoint', () => {
  const base = 'http://127.0.0.1:1777/';
  assert.deepEqual(parser.outputURL('/api/output/PLC%20715.xml', base), { href: base + 'api/output/PLC%20715.xml', name: 'PLC 715.xml' });
  for (const value of ['https://example.com/api/output/a.xml', 'http://127.0.0.1:1778/api/output/a.xml', '/api/templates', '/api/output/../secret.xml', '/api/output/%2e%2e%2fsecret.xml', '/api/output/a%5cb.xml', '/api/output/a.xls', '/api/output/a.xml?query=1', 'javascript:alert(1)', 'http://user:pass@127.0.0.1:1777/api/output/a.xml']) assert.throws(() => parser.outputURL(value, base), undefined, value);
});
// XML point lists, junction markers and port identities are parsed without inferring missing connections.
test('point geometry and endpoint port identity preserve saved XML values', () => {
  assert.deepEqual(parser.points('(1,2);*(3,-4); (5,6);'), [{x:1,y:2,junction:false},{x:3,y:-4,junction:true},{x:5,y:6,junction:false}]);
  assert.deepEqual(parser.points('(1,2);garbage(3,4);'), []);
  assert.deepEqual(parser.endpoint('150|True|i03|0,0,50,60'), {raw:'150|True|i03|0,0,50,60',block:'150',input:'True',port:'i03',bounds:'0,0,50,60'});
  assert.equal(parser.parameters('[TEXT]=A=B\n[PL]=(1,2);').TEXT, 'A=B');
});
// The preview viewport encloses saved blocks and link points even outside the declared canvas.
test('fit uses actual geometry rather than declared page size', () => {
  assert.deepEqual(parser.bounds({blocks:[{x:10,y:20,width:100,height:50}],primitives:[],links:[{points:[{x:-20,y:30},{x:10,y:30}]}]}), {x:-50,y:-10,width:190,height:110});
});
