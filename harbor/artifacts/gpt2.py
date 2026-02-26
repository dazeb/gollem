#!/usr/bin/env python3
import sys,re,numpy as n
ck,bf,txt=sys.argv[1],sys.argv[2],sys.argv[3]
V,T,L,H,C=50257,1024,12,12,768;S=C//H
x=n.fromfile(ck,n.float32);p=0
def t(k,*s):
 global p
 y=x[p:p+k];p+=k
 return y.reshape(s) if s else y
wte=t(V*C,V,C);wpe=t(T*C,T,C)
ln1w=t(L*C,L,C);ln1b=t(L*C,L,C)
qkvw=t(L*3*C*C,L,3*C,C);qkvb=t(L*3*C,L,3*C)
apw=t(L*C*C,L,C,C);apb=t(L*C,L,C)
ln2w=t(L*C,L,C);ln2b=t(L*C,L,C)
fcw=t(L*4*C*C,L,4*C,C);fcb=t(L*4*C,L,4*C)
fpw=t(L*C*4*C,L,C,4*C);fpb=t(L*C,L,C)
lnfw=t(C);lnfb=t(C)
bs=list(range(33,127))+list(range(161,173))+list(range(174,256));cs=bs[:]
for b in range(256):
 if b not in bs:cs.append(256+len(cs)-len(bs));bs.append(b)
be={b:chr(c) for b,c in zip(bs,cs)};bd={v:k for k,v in be.items()}
m=[tuple(s.split()) for s in open(bf,encoding='utf8').read().splitlines()[1:] if s]
v=[be[b] for b in bs]+[a+b for a,b in m]+['<|endoftext|>'];e={s:i for i,s in enumerate(v)};r={p:i for i,p in enumerate(m)};c={}
pt=re.compile(r"'s|'t|'re|'ve|'m|'ll|'d| ?[A-Za-z]+| ?\\d+| ?[^\\sA-Za-z\\d]+|\\s+(?!\\S)|\\s+")
def bpe(s):
 z=c.get(s)
 if z:return z
 w=list(s)
 while 1:
  pr=[(r.get((w[i],w[i+1]),10**9),i) for i in range(len(w)-1)]
  j=min(pr)[1] if pr else -1
  if j<0 or r.get((w[j],w[j+1]),10**9)==10**9:break
  a,b=w[j],w[j+1];u=[];i=0
  while i<len(w):
   if i<len(w)-1 and w[i]==a and w[i+1]==b:u.append(a+b);i+=2
   else:u.append(w[i]);i+=1
  w=u
  if len(w)<2:break
 z=' '.join(w);c[s]=z;return z
def enc(s):
 o=[]
 for t in pt.findall(s):
  t=''.join(be[b] for b in t.encode())
  o+= [e[q] for q in bpe(t).split(' ')]
 return o
def dec(a):return bytes(bd[ch] for i in a for ch in v[i]).decode('utf8','replace')
def ln(a,g,b):
 u=a.mean();d=((a-u)**2).mean()
 return (a-u)/n.sqrt(d+1e-5)*g+b
K=n.empty((L,T,H,S),n.float32);Va=n.empty((L,T,H,S),n.float32);iq=1/n.sqrt(S)
def step(tok,pos):
 z=wte[tok]+wpe[pos]
 for l in range(L):
  y=ln(z,ln1w[l],ln1b[l]);q=qkvw[l].dot(y)+qkvb[l]
  Q=q[:C].reshape(H,S);K[l,pos]=q[C:2*C].reshape(H,S);Va[l,pos]=q[2*C:].reshape(H,S)
  ks=K[l,:pos+1];vs=Va[l,:pos+1];s=n.einsum('thd,hd->ht',ks,Q)*iq;s-=s.max(1,keepdims=1);n.exp(s,out=s);s/=s.sum(1,keepdims=1)
  z=z+apw[l].dot(n.einsum('ht,thd->hd',s,vs).reshape(C))+apb[l]
  y=ln(z,ln2w[l],ln2b[l]);u=fcw[l].dot(y)+fcb[l];u=.5*u*(1+n.tanh(.7978845608*(u+.044715*u*u*u)))
  z=z+fpw[l].dot(u)+fpb[l]
 y=ln(z,lnfw,lnfb)
 return wte.dot(y)
ids=enc(txt) or [50256]
if len(ids)>T-20:ids=ids[-(T-20):]
for i,tok in enumerate(ids):lg=step(tok,i)
out=[]
for i in range(20):
 k=int(lg.argmax());out.append(k);lg=step(k,len(ids)+i)
sys.stdout.write(txt+dec(out))