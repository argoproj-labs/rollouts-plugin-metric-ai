## Canary Analysis Process Flow

This diagram shows the complete flow from canary deployment through AI analysis to automated PR creation:

```mermaid
flowchart TD
    Start([Argo Rollouts Canary Deployment]) --> Canary[Deploy Canary v2<br/>alongside Stable v1]
    Canary --> Traffic[Route Traffic<br/>10% to Canary<br/>90% to Stable]
    Traffic --> Analysis[Argo Rollouts Analysis<br/>runs Analysis Template]
    
    Analysis --> Plugin{Metric AI Plugin<br/>argoproj-labs/metric-ai}
    
    Plugin --> A2ACall[Single A2A call:<br/>POST /a2a/analyze<br/>with pod selectors]
    
    A2ACall --> K8sAgent[Kubernetes AI Agent<br/>in cluster]
    
    K8sAgent --> AgentTools[Agent uses tools:<br/>1. rollout status<br/>2. canary metrics<br/>3. canary diagnostics<br/>pod logs and events<br/>4. pause rollout]
    
    AgentTools --> AgentAnalyze[Agent analyzes with<br/>the configured LLM]
    
    AgentAnalyze --> RootCause[Identify Root Cause]
    
    RootCause --> NeedsFix{Rollback with<br/>a repo configured?}
    
    NeedsFix -->|Yes| Delegate[Delegate remediation over A2A<br/>runs in the background]
    NeedsFix -->|No| Response
    
    Delegate --> Remediation[Remediation agent sandbox:<br/>fix, verify with tests,<br/>open PR or issue]
    Remediation --> PRLink[PR / issue link<br/>published via activity events]
    PRLink --> Response
    
    Response[Return Analysis Response:<br/>- Root Cause<br/>- Promote Decision<br/>- Confidence Level]
    
    Response --> PluginDecision{Plugin Decision}
    
    PluginDecision -->|Promote| Promote[Promote Canary<br/>to 100%]
    PluginDecision -->|Abort| Abort[Abort Rollout<br/>Rollback to Stable]
    
    Promote --> Complete([Deployment Complete])
    Abort --> Complete
    
    style Start fill:#e1f5e1
    style Complete fill:#e1f5e1
    style K8sAgent fill:#fff3cd
    style Plugin fill:#d1ecf1
    style Delegate fill:#f8d7da
    style Remediation fill:#f8d7da
    style PRLink fill:#d4edda
    style AgentTools fill:#cfe2ff
```

## System Architecture

This diagram shows all the components, services, and their interactions:

```mermaid
graph TB
    subgraph "Kubernetes Cluster"
        subgraph "openshift-gitops"
            ArgoController[Argo Rollouts Controller<br/>with metric-ai plugin]
            AgentService[Service: kubernetes-agent<br/>Port 8080]
            AgentPod[kubernetes-agent<br/>analysis, LLM, tools]
            RemediationSvc[remediation-agent<br/>A2A remediation service]
        end
        
        subgraph "Workload namespace, e.g. quarkus-demo"
            StablePods[Stable Pods v1]
            CanaryPods[Canary Pods v2]
        end
        
        subgraph "remediation, sandbox namespace"
            Sandbox[Sandbox pods<br/>coding agent, no cluster access]
        end
        
        subgraph "Infrastructure"
            K8sAPI[Kubernetes API]
            MetricsServer[Metrics Server]
        end
    end
    
    subgraph "External Services"
        GitHub[GitHub<br/>source, PRs, issues]
        LLM[LLM API<br/>OpenAI-compatible or Gemini]
    end
    
    subgraph "Developer Workflow"
        DevPush[Developer<br/>push code]
        CICD[CI/CD Pipeline<br/>build and deploy]
        PRReview[PR Review<br/>review agent fix]
    end
    
    %% Deployment Flow
    DevPush -->|git push| GitHub
    GitHub -->|webhook| CICD
    CICD -->|deploy| ArgoController
    
    %% Canary Rollout Flow
    ArgoController -->|creates| StablePods
    ArgoController -->|creates| CanaryPods
    
    %% Plugin Analysis Flow
    ArgoController -->|POST /a2a/analyze| AgentService
    
    %% Agent Operations
    AgentService --> AgentPod
    AgentPod -->|queries, pauses| K8sAPI
    AgentPod -->|get logs| StablePods
    AgentPod -->|get logs| CanaryPods
    AgentPod -->|get metrics| MetricsServer
    AgentPod -->|LLM calls| LLM
    AgentPod -->|async A2A after rollback| RemediationSvc
    
    %% Remediation Flow
    RemediationSvc -->|creates| Sandbox
    Sandbox -->|git push, PR or issue| GitHub
    
    %% Infrastructure
    K8sAPI -.->|manages| StablePods
    K8sAPI -.->|manages| CanaryPods
    K8sAPI -.->|manages| AgentPod
    
    %% PR Flow
    GitHub -->|notification| PRReview
    PRReview -->|merge| GitHub
    
    %% Styling
    style ArgoController fill:#326ce5,color:#fff
    style AgentPod fill:#ff6f00,color:#fff
    style RemediationSvc fill:#00c853,color:#fff
    style GitHub fill:#24292e,color:#fff
    style LLM fill:#4285f4,color:#fff
    style K8sAPI fill:#326ce5,color:#fff
    
    classDef stable fill:#4caf50,color:#fff
    classDef canary fill:#ff9800,color:#fff
    classDef external fill:#9e9e9e,color:#fff
    
    class StablePods stable
    class CanaryPods canary
    class GitHub,LLM external
```
