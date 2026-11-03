import type { TeamHostBindings } from "./teamBindings";
import type { TeamSecretsView, TeamView } from "../generated/desktopContract.generated";

// Opt-in browser interaction fixture. It stores metadata only and does not
// authorize a command, persist a credential or manufacture model responses.
export function makeTeamSecretMock(team:TeamView):Pick<TeamHostBindings,"GetTeamSecrets"|"SaveTeamSecret"> {
  const view:TeamSecretsView={available:true,revision:0,rosterRevision:team.roster.revision,secrets:[],requests:[],retiredCount:0};
  const snapshot=()=>structuredClone({...view,rosterRevision:team.roster.revision});
  return {
    GetTeamSecrets:async()=>snapshot(),
    SaveTeamSecret:async req=>{
      if(req.expectedRevision!==view.revision||req.expectedRosterRevision!==team.roster.revision)return {outcome:"rejected",reason:"conflict",view:snapshot()};
      const index=view.secrets.findIndex(secret=>secret.name===req.name);
      if(req.operation==="forget_retired") {
        if(!req.confirmForget)return {outcome:"rejected",reason:"invalid",view:snapshot()};
        view.retiredCount=0;
      } else if(req.operation==="delete") {
        if(index<0)return {outcome:"rejected",reason:"target_invalid",view:snapshot()};
        view.secrets.splice(index,1);view.retiredCount++;
      } else if(req.operation==="decline") {
        view.requests=view.requests.filter(request=>request.id!==req.secretRequestId);
      } else {
        if(req.operation==="save"&&req.value.length<4)return {outcome:"rejected",reason:"invalid",view:snapshot()};
        const secret={name:req.name,purpose:req.purpose,agents:[...req.agents],updatedAt:new Date().toISOString()};
        if(index<0)view.secrets.push(secret);else {view.secrets[index]=secret;if(req.operation==="save")view.retiredCount++;}
      }
      view.revision++;return {outcome:"applied",view:snapshot()};
    },
  };
}
