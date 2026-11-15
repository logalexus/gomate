// Remember recent revisions, including tombstones, so a slow poll cannot undo
// a local edit/delete. Rendered history remains capped independently of this cache.
export function mergeChatMessages(current, incoming, versions, {replace=false, insert=true, prepend=false}={}) {
 for(const m of [...current,...incoming]) {
  const old=versions.get(m.id);
  if(!old||(m.revision||0)>=(old.revision||0)) {
   versions.delete(m.id);versions.set(m.id,m);
  }
 }
 const ids=new Set(replace?[]:current.map(m=>m.id));
 if(insert)for(const m of incoming)ids.add(m.id);
 const result=[...ids].map(id=>versions.get(id)).filter(m=>m&&!m.deleted).sort((a,b)=>a.id-b.id);
 // Protect every loaded row even after a large batch of changes to other rows.
 const keep=new Set(result.map(m=>m.id));
 for(const id of versions.keys()) {if(versions.size<=1500)break;if(!keep.has(id))versions.delete(id)}
 return prepend?result.slice(0,500):result.slice(-500);
}
