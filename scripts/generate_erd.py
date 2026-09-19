#!/usr/bin/env python3
"""Generate editable diagrams.net ERD and SVG from the committed SQL migration."""
from pathlib import Path
import re
import html
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs/diagrams'
SQL = ROOT / 'migrations/001_init.sql'
W, H = 1500, 1140
NAVY, INK, MUTED, TEAL, BORDER = '#142B45', '#253E55', '#6A7D8E', '#087F8C', '#D9E3EB'
xml = ET.Element('mxfile', host='app.diagrams.net', agent='Hospital Middleware', version='24.7.17')
diag = ET.SubElement(xml, 'diagram', id='hospital-erd', name='Hospital data model')
model = ET.SubElement(diag, 'mxGraphModel', dx=str(W), dy=str(H), grid='1', gridSize='10', guides='1', tooltips='1', connect='1', arrows='1', fold='1', page='1', pageScale='1', pageWidth=str(W), pageHeight=str(H), math='0', shadow='0', background='#FFFFFF')
root = ET.SubElement(model, 'root')
ET.SubElement(root, 'mxCell', id='0')
ET.SubElement(root, 'mxCell', id='1', parent='0')
svg = [f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" viewBox="0 0 {W} {H}">', '<rect width="1500" height="1140" fill="white"/>']
serial = 0

def cell(value, x, y, w, h, style, ident=None):
    global serial
    serial += 1
    c = ET.SubElement(root, 'mxCell', id=ident or f'n{serial}', value=value, style=style, vertex='1', parent='1')
    ET.SubElement(c, 'mxGeometry', x=str(x), y=str(y), width=str(w), height=str(h), **{'as': 'geometry'})
    return c

def rect(x,y,w,h,fill,stroke='none',radius=0,ident=None):
    cell('',x,y,w,h,f'rounded={1 if radius else 0};arcSize=12;whiteSpace=wrap;html=0;fillColor={fill};strokeColor={stroke};strokeWidth=1;',ident)
    svg.append(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{radius}" fill="{fill}" stroke="{stroke}"/>')

def text(value,x,y,w,h=24,size=15,color=INK,bold=False,align='left',font='Helvetica'):
    cell(value,x,y,w,h,f'text;html=0;strokeColor=none;fillColor=none;align={align};verticalAlign=middle;whiteSpace=wrap;overflow=hidden;rounded=0;fontFamily={font};fontSize={size};fontColor={color};fontStyle={1 if bold else 0};spacing=0;')
    sx=x if align=='left' else x+w/2
    anchor='start' if align=='left' else 'middle'
    lines=value.split('\n')
    for i,line in enumerate(lines):
        sy=y+h/2-(len(lines)-1)*size*0.65+i*size*1.3+size*.35
        svg.append(f'<text x="{sx}" y="{sy}" text-anchor="{anchor}" font-family="{font}, Arial, sans-serif" font-size="{size}" font-weight="{700 if bold else 400}" fill="{color}">{html.escape(line)}</text>')

def line(x1,y1,x2,y2,color=BORDER):
    svg.append(f'<path d="M{x1} {y1} L{x2} {y2}" fill="none" stroke="{color}"/>')
    c=ET.SubElement(root,'mxCell',id=f'line{len(svg)}',value='',style=f'edgeStyle=none;endArrow=none;html=0;strokeColor={color};',edge='1',parent='1')
    g=ET.SubElement(c,'mxGeometry',relative='1',**{'as':'geometry'})
    ET.SubElement(g,'mxPoint',x=str(x1),y=str(y1),**{'as':'sourcePoint'})
    ET.SubElement(g,'mxPoint',x=str(x2),y=str(y2),**{'as':'targetPoint'})

def fields(table):
    source=SQL.read_text()
    block=re.search(r'CREATE TABLE(?: IF NOT EXISTS)?\s+'+table+r'\s*\((.*?)\n\);',source,re.S|re.I)
    if not block: raise ValueError(f'Missing table {table}')
    rows=[]
    for m in re.finditer(r'(?:^|,)\s*(\w+)\s+(BIGSERIAL|BIGINT|TEXT|DATE|TIMESTAMPTZ|VARCHAR\(\d+\))([^,]*)', block.group(1), re.I):
        name,typ,tail=m.groups()
        marker='PK' if 'PRIMARY KEY' in tail.upper() else 'FK' if 'REFERENCES' in tail.upper() else ''
        null='REQUIRED' if 'NOT NULL' in tail.upper() or marker=='PK' else 'NULL'
        rows.append((marker,name,typ.lower(),null))
    return rows

text('HOSPITAL MIDDLEWARE',90,48,900,24,13,TEAL,True)
text('A clear boundary for every hospital.',90,87,1300,56,38,NAVY,True)
text('Entity relationship diagram  /  PostgreSQL persistence model',92,151,1200,28,18,MUTED)
rect(90,197,1320,2,BORDER)


def table(name,label,subtitle,x,y,w,constraint):
    rows=fields(name)
    rowh=34
    h=105+len(rows)*rowh+64
    rect(x,y,w,h,'#FFFFFF',BORDER,10,name)
    rect(x,y,w,70,NAVY)
    text(label,x+22,y+10,w-44,30,23,'#FFFFFF',True)
    text(subtitle,x+22,y+42,w-44,19,12,'#B8CEDC')
    rect(x+1,y+70,w-2,35,'#F0F5F8')
    text('KEY',x+16,y+76,42,23,10,MUTED,True)
    text('COLUMN',x+69,y+76,w-277,23,10,MUTED,True)
    text('TYPE',x+w-176,y+76,82,23,10,MUTED,True)
    text('NULLABLE',x+w-87,y+76,77,23,10,MUTED,True)
    for i,(marker,col,typ,null) in enumerate(rows):
        yy=y+105+i*rowh
        if i%2==1: rect(x+1,yy,w-2,rowh,'#FAFCFD')
        if marker:
            rect(x+14,yy+7,34,20,'#DFF2F3' if marker=='FK' else '#E8EEF4',radius=4)
            text(marker,x+14,yy+7,34,20,10,TEAL if marker=='FK' else NAVY,True,'center')
        text(col,x+69,yy+4,w-250,26,14,INK,marker!='')
        text(typ,x+w-176,yy+4,88,26,12,MUTED)
        text('Yes' if null=='NULL' else 'No',x+w-87,yy+4,70,26,12,MUTED)
    yy=y+105+len(rows)*rowh
    line(x,yy,x+w,yy)
    text('UNIQUE',x+18,yy+9,72,19,10,TEAL,True)
    text(constraint,x+18,yy+29,w-36,23,13,INK)
    return h

hh=table('hospitals','Hospital','hospitals  ·  tenant registry',90,245,480,'code')
sh=table('staff','Staff','staff  ·  authenticated users',90,625,480,'(hospital_id, username)')
ph=table('patients','Patient','patients  ·  hospital-scoped records',860,245,550,'(hospital_id, patient_hn)')

# Native ER endpoints: exactly one hospital, zero or many dependent records.
def relationship(ident,source,target,points,label,lx,ly,lw):
    c=ET.SubElement(root,'mxCell',id=ident,value='',style='edgeStyle=orthogonalEdgeStyle;rounded=0;html=0;strokeColor=#087F8C;strokeWidth=2;startArrow=ERmandOne;startFill=0;startSize=16;endArrow=ERzeroToMany;endFill=0;endSize=20;',edge='1',parent='1')
    c.set('source', source)
    c.set('target', target)
    anchors = 'exitX=0.5;exitY=1;entryX=0.5;entryY=0;' if target=='staff' else f'exitX=1;exitY={88/hh};entryX=0;entryY={88/ph};'
    c.set('style', c.get('style') + anchors)
    g=ET.SubElement(c,'mxGeometry',relative='1',**{'as':'geometry'})
    ET.SubElement(g,'mxPoint',x=str(points[0][0]),y=str(points[0][1]),**{'as':'sourcePoint'})
    ET.SubElement(g,'mxPoint',x=str(points[-1][0]),y=str(points[-1][1]),**{'as':'targetPoint'})
    a=ET.SubElement(g,'Array',**{'as':'points'})
    for px,py in points[1:-1]: ET.SubElement(a,'mxPoint',x=str(px),y=str(py))
    svg.append('<path d="'+' '.join(('M' if i==0 else 'L')+f'{px} {py}' for i,(px,py) in enumerate(points))+'" fill="none" stroke="#087F8C" stroke-width="2"/>')
    def svgline(x1,y1,x2,y2,color):
        svg.append(f'<path d="M{x1} {y1} L{x2} {y2}" fill="none" stroke="{color}" stroke-width="2"/>')
    x,y=points[0]; ex,ey=points[-1]
    if x==ex:
        for d in (10,17): svgline(x-8,y+d,x+8,y+d,TEAL)
        svgline(ex,ey,ex-9,ey-18,TEAL);svgline(ex,ey,ex+9,ey-18,TEAL)
        svg.append(f'<circle cx="{ex}" cy="{ey-27}" r="5" fill="white" stroke="{TEAL}" stroke-width="2"/>')
    else:
        for d in (10,17): svgline(x+d,y-8,x+d,y+8,TEAL)
        svgline(ex,ey,ex-18,ey-9,TEAL);svgline(ex,ey,ex-18,ey+9,TEAL)
        svg.append(f'<circle cx="{ex-27}" cy="{ey}" r="5" fill="white" stroke="{TEAL}" stroke-width="2"/>')
    text(label,lx,ly,lw,42,13,TEAL,True,'center')

relationship('hospital-staff','hospitals','staff',[(330,245+hh),(330,625)],'1 hospital\n0..* staff',350,535,180)
relationship('hospital-patients','hospitals','patients',[(570,333),(860,333)],'1 hospital  →  0..* patients',585,275,260)

rect(90,960,1320,104,'#F0F7F8',radius=8)
text('TENANT ISOLATION',112,975,280,22,11,TEAL,True)
text('Staff access is scoped by staff.hospital_id.',112,1006,600,25,16,NAVY,True)
text('Patient identifiers may repeat across hospitals.\nHN is unique only within its hospital; national_id and passport_id are not unique.',735,985,648,58,14,INK)
text('PK  Primary key     FK  Foreign key     Nullable: Yes = SQL NULL permitted',90,1083,890,23,12,MUTED)
text('Source: migrations/001_init.sql',1050,1083,360,23,12,MUTED)
svg.append('</svg>')
OUT.mkdir(parents=True,exist_ok=True)
ET.indent(xml,space='  ')
ET.ElementTree(xml).write(OUT/'hospital-erd.drawio',encoding='utf-8',xml_declaration=True)
(OUT/'hospital-erd.svg').write_text('\n'.join(svg))
print(f'Wrote {W} × {H} ERD to {OUT}')
