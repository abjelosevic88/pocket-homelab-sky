#!/usr/bin/env python3
"""Builds internal/catalog/data/*.json from the d3-celestial data files
(https://github.com/ofrohn/d3-celestial/tree/master/data, BSD 3-Clause;
stars from XHIP / Hipparcos, names from the IAU and the HD/HIP cross index,
Messier data from SEDS, Milky Way outline by Jose R. Vieira).

Usage: python3 scripts/catalog.py DIR_WITH_D3_CELESTIAL_DATA
"""
import json, os, sys

src = sys.argv[1]
out = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'internal', 'catalog', 'data')
os.makedirs(out, exist_ok=True)

def ra(x):
    return round(x % 360, 4)

def load(name):
    with open(os.path.join(src, name), encoding='utf-8') as f:
        return json.load(f)

def dump(name, v):
    with open(os.path.join(out, name), 'w', encoding='utf-8') as f:
        json.dump(v, f, ensure_ascii=False, separators=(',', ':'))
    print(name, os.path.getsize(os.path.join(out, name)))

names = load('starnames.json')
stars = []
for f in load('stars.6.json')['features']:
    p = f['properties']
    lon, lat = f['geometry']['coordinates']
    try:
        bv = round(float(p.get('bv') or 0.6), 2)
    except ValueError:
        bv = 0.6
    hip = int(f['id'])
    n = names.get(str(hip), {})
    stars.append([ra(lon), round(lat, 4), round(p['mag'], 2), bv, hip, n.get('name', ''), n.get('bayer', '') or n.get('flam', ''), n.get('c', '')])
stars.sort(key=lambda s: s[2])
dump('stars.json', stars)

cons = {f['id']: f['properties'] for f in load('constellations.json')['features']}
lines = {f['id']: f['geometry']['coordinates'] for f in load('constellations.lines.json')['features']}
out_c = []
for cid, p in cons.items():
    ls = [[[ra(a), round(b, 3)] for a, b in seg] for seg in lines.get(cid, [])]
    d = p.get('display', [0, 0, 0])
    out_c.append({'id': cid, 'name': p['name'], 'gen': p.get('gen', ''), 'rank': int(p.get('rank', 3)), 'label': [ra(d[0]), d[1]], 'lines': ls})
out_c.sort(key=lambda c: c['name'])
dump('constellations.json', out_c)

bounds = []
for f in load('constellations.bounds.json')['features']:
    g = f['geometry']
    polys = g['coordinates'] if g['type'] == 'MultiPolygon' else [g['coordinates']]
    bounds.append({'id': f['id'], 'polys': [[[ra(a), round(b, 4)] for a, b in ring] for poly in polys for ring in poly]})
dump('bounds.json', bounds)

mess = []
for f in load('messier.json')['features']:
    p = f['properties']
    lon, lat = f['geometry']['coordinates']
    mess.append({'id': p['name'], 'ngc': p.get('desig', ''), 'name': p.get('alt', ''), 'type': p.get('type', ''), 'mag': p.get('mag'), 'dim': p.get('dim', ''), 'ra': ra(lon), 'dec': round(lat, 4)})
mess.sort(key=lambda m: int(m['id'][1:]))
dump('messier.json', mess)

mw = []
for f in load('mw.json')['features']:
    if f['id'] not in ('ol1', 'ol2', 'ol3'):
        continue
    step = {'ol1': 3, 'ol2': 3, 'ol3': 2}[f['id']]
    rings = []
    for poly in f['geometry']['coordinates']:
        for ring in poly:
            pts = ring[::step]
            if len(pts) >= 4:
                rings.append([[round(a % 360, 1), round(b, 1)] for a, b in pts])
    mw.append({'id': f['id'], 'rings': rings})
dump('milkyway.json', mw)
