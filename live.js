(()=>{
 'use strict';
 const $=id=>document.getElementById(id);
 const out=(id,obj)=>{$(id).textContent=typeof obj==='string'?obj:JSON.stringify(obj,null,2)};
 async function api(path,opts={}){
  const response=await fetch(path,{credentials:'same-origin',cache:'no-store',...opts,headers:{...(opts.body?{'Content-Type':'application/json'}:{}),...(opts.headers||{})}});
  const payload=await response.json().catch(()=>({error:`HTTP ${response.status}`}));
  if(!response.ok)throw Object.assign(new Error(payload.error||`HTTP ${response.status}`),{payload});
  return payload;
 }
 const post=(path,data)=>api(path,{method:'POST',body:JSON.stringify(data)});
 const fail=(id,e)=>out(id,e.payload||{error:e.message});
 function unlock(show){$('live-lock').hidden=!show;$('live-status').textContent=show?'LOCAL · AUTH REQUIRED':'LOCAL · AUTHENTICATED';$('live-status').classList.toggle('connected',!show);$('runtime-mode').textContent=show?'LIVE SERVICES · LOCKED':'LIVE SERVICES · LOCAL';}
 async function refresh(){
  try{
   const [state,verification,events]=await Promise.all([api('/api/state'),api('/api/verify'),api('/api/events')]);
   unlock(false);
   $('live-integrity').textContent=verification.verified?'VALID':'FAILED';$('live-events').textContent=String(state.total);$('live-allowed').textContent=String(state.allowed);$('live-denied').textContent=String(state.denied);
   $('model-status').textContent=state.model_configured?'Local model endpoint configured (health unverified)':'Local model endpoint not configured';
   $('event-count').textContent=`${state.total} total events, showing ${events.events.length}`;
   const list=$('live-event-list');list.replaceChildren();
   for(const ev of [...events.events].reverse().slice(0,30)){
    const row=document.createElement('div');row.className='live-event';
    const info=document.createElement('div');const name=document.createElement('strong');name.textContent=`${ev.kind.toUpperCase()} · ${ev.tool||ev.decision}`;
    const small=document.createElement('small');small.textContent=`${ev.decision} · ${ev.path||'—'} · ${ev.time}`;
    info.append(name,small);const button=document.createElement('button');button.textContent='Inspect';button.type='button';button.addEventListener('click',()=>{$('replay-id').value=ev.id;$('replay-id').scrollIntoView({behavior:'smooth',block:'center'});});
    row.append(info,button);list.append(row);
   }
   if(!events.events.length)list.textContent='No live events. Execute a tool to start your evidence ledger.';
  }catch(e){if(e.message==='UNAUTHORIZED'){unlock(true);return};out('proof-output',e.payload||e.message)}
 }
 $('unlock-form').addEventListener('submit',async e=>{e.preventDefault();try{await post('/api/login',{token:$('access-token').value.trim()});$('access-token').value='';out('auth-output','Authenticated.');await refresh()}catch(err){fail('auth-output',err)}});
 $('live-logout').addEventListener('click',async()=>{await post('/api/logout',{}).catch(()=>{});unlock(true)});
 const tool=()=>({tool:$('tool').value,path:$('tool-path').value,content:$('tool').value==='write'?$('tool-content').value:''});
 $('tool-evaluate').addEventListener('click',async()=>{try{out('tool-output',await post('/api/evaluate',tool()))}catch(e){fail('tool-output',e)}});
 $('tool-run').addEventListener('click',async()=>{try{out('tool-output',await post('/api/execute',tool()))}catch(e){fail('tool-output',e)}finally{await refresh()}});
 $('proof-verify').addEventListener('click',async()=>{try{out('proof-output',await api('/api/verify'))}catch(e){fail('proof-output',e)}});
 $('events-refresh').addEventListener('click',refresh);
 $('replay-run').addEventListener('click',async()=>{try{out('replay-output',await api(`/api/replay/${encodeURIComponent($('replay-id').value.trim())}`))}catch(e){fail('replay-output',e)}});
 $('feedback-save').addEventListener('click',async()=>{try{out('feedback-output',await post('/api/feedback',{prompt:$('feedback-prompt').value,chosen:$('feedback-chosen').value,rejected:$('feedback-rejected').value}));await refresh()}catch(e){fail('feedback-output',e)}});
 $('model-run').addEventListener('click',async()=>{try{out('model-output',await post('/api/infer',{prompt:$('model-prompt').value}));await refresh()}catch(e){fail('model-output',e)}});
 // Browser-only file preview is intentionally not treated as authenticated or live.
 refresh();setInterval(()=>{if($('live-lock').hidden)refresh()},8000);
})();
