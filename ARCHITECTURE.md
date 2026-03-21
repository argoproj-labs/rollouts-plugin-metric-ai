## Canary Analysis Process Flow

This diagram shows the complete flow from canary deployment through AI analysis to automated PR creation:

```mermaid
flowchart TD
    Start([Argo Rollouts Canary Deployment]) --> Canary[Deploy Canary v2<br/>alongside Stable v1]
    Canary --> Traffic[Route Traffic<br/>10% to Canary<br/>90% to Stable]
    Traffic --> Analysis[Argo Rollouts Analysis<br/>runs Analysis Template]
    
    Analysis --> Plugin{Metric AI Plugin<br/>argoproj-labs/metric-ai}
    
    Plugin --> Monitor[Monitor Metrics:<br/>- Success Rate<br/>- Error Count<br/>- Response Time]
    
    Monitor --> Threshold{Metrics<br/>Meet Threshold?}
    
    Threshold -->|Yes| Success[Return: Success<br/>Promote Canary]
    Threshold -->|No| AIAnalysis[Call AI Mode<br/>Ask Gemini to analyze]
    
    AIAnalysis --> GatherData[Gather Context:<br/>- Pod logs<br/>- K8s events<br/>- Metrics data<br/>- Rollout info]
    
    GatherData --> DecideMode{AI Decision:<br/>Use Agent?}
    
    DecideMode -->|"Yes (complex)"| A2ACall[Agent-to-Agent Call<br/>POST /a2a/analyze]
    DecideMode -->|"No (simple)"| DirectAI[Direct Gemini Analysis]
    
    A2ACall --> K8sAgent[Kubernetes AI Agent<br/>in cluster]
    
    K8sAgent --> AgentTools[Agent uses tools:<br/>1. debug_kubernetes_pod<br/>2. get_pod_logs<br/>3. get_kubernetes_events<br/>4. get_pod_metrics<br/>5. inspect_kubernetes_resources]
    
    AgentTools --> AgentAnalyze[Agent analyzes with<br/>Gemini 2.0 Flash]
    
    AgentAnalyze --> SearchKnown[Search Google for<br/>known issues]
    
    SearchKnown --> RootCause[Identify Root Cause]
    
    RootCause --> NeedsFix{Code Fix<br/>Needed?}
    
    NeedsFix -->|Yes| CreatePR[create_github_pr tool:<br/>1. Clone repo<br/>2. Create branch<br/>3. Apply fixes<br/>4. Commit & push<br/>5. Create PR]
    
    NeedsFix -->|No| Recommend[Provide<br/>Recommendations]
    
    CreatePR --> PRCreated[GitHub PR Created<br/>with analysis]
    Recommend --> Response
    PRCreated --> Response
    
    DirectAI --> Response[Return Analysis Response:<br/>- Root Cause<br/>- Promote Decision<br/>- Confidence Level<br/>- PR Link if created]
    
    Response --> PluginDecision{Plugin Decision}
    
    PluginDecision -->|Promote| Promote[Promote Canary<br/>to 100%]
    PluginDecision -->|Abort| Abort[Abort Rollout<br/>Rollback to Stable]
    
    Success --> Promote
    
    Promote --> Complete([Deployment Complete])
    Abort --> Complete
    
    style Start fill:#e1f5e1
    style Complete fill:#e1f5e1
    style K8sAgent fill:#fff3cd
    style Plugin fill:#d1ecf1
    style CreatePR fill:#f8d7da
    style PRCreated fill:#d4edda
    style AgentTools fill:#cfe2ff
```

## System Architecture

This diagram shows all the components, services, and their interactions:

```mermaid
graph TB
    subgraph "Kubernetes Cluster"
        subgraph "Argo Rollouts Namespace"
            ArgoController[Argo Rollouts Controller<br/>📦 Manages Canary]
            
            subgraph "Application Deployment"
                StablePods[Stable Pods v1<br/>🟢 90% traffic]
                CanaryPods[Canary Pods v2<br/>🟡 10% traffic]
            end
            
            subgraph "Rollouts Plugin Metric AI"
                PluginPod[Argo Rollouts controller<br/>🔌 metric plugin binary]
            end
            
            subgraph "Kubernetes AI Agent"
                AgentPod[Agent Pod<br/>🤖 Java ADK Server]
                AgentService[Service: kubernetes-agent<br/>🌐 Port 8080]
            end
        end
        
        subgraph "Infrastructure"
            K8sAPI[Kubernetes API<br/>📡 Cluster Control]
            MetricsServer[Metrics Server<br/>📊 Resource Metrics]
            CoreDNS[CoreDNS<br/>🔍 Service Discovery]
        end
    end
    
    subgraph "External Services"
        GitHub[GitHub<br/>📝 Source Code]
        GitHubAPI[GitHub API<br/>🔧 PR Creation]
        GeminiAPI[Google Gemini API<br/>🧠 AI Analysis]
        GoogleSearch[Google Search<br/>🔎 Known Issues]
    end
    
    subgraph "Developer Workflow"
        DevPush[Developer<br/>👨‍💻 Push Code]
        CICD[CI/CD Pipeline<br/>⚙️ Build & Deploy]
        PRReview[PR Review<br/>👀 Review Agent Fix]
    end
    
    %% Deployment Flow
    DevPush -->|git push| GitHub
    GitHub -->|webhook| CICD
    CICD -->|deploy| ArgoController
    
    %% Canary Rollout Flow
    ArgoController -->|creates| StablePods
    ArgoController -->|creates| CanaryPods
    ArgoController -->|triggers| PluginPod
    
    %% Plugin Analysis Flow
    PluginPod -->|reads metrics| CanaryPods
    PluginPod -->|queries| K8sAPI
    PluginPod -->|calls API| GeminiAPI
    PluginPod -->|A2A POST /a2a/analyze| AgentService
    
    %% Agent Operations
    AgentService --> AgentPod
    AgentPod -->|queries| K8sAPI
    AgentPod -->|get logs| CanaryPods
    AgentPod -->|get events| K8sAPI
    AgentPod -->|get metrics| MetricsServer
    AgentPod -->|AI analysis| GeminiAPI
    AgentPod -->|search issues| GoogleSearch
    AgentPod -->|create PR| GitHubAPI
    
    %% Infrastructure
    K8sAPI -.->|manages| StablePods
    K8sAPI -.->|manages| CanaryPods
    K8sAPI -.->|manages| PluginPod
    K8sAPI -.->|manages| AgentPod
    CoreDNS -.->|resolves| AgentService
    
    %% PR Flow
    GitHubAPI -->|creates PR| GitHub
    GitHub -->|notification| PRReview
    PRReview -->|merge| GitHub
    
    %% Styling
    style ArgoController fill:#326ce5,color:#fff
    style PluginPod fill:#00c853,color:#fff
    style AgentPod fill:#ff6f00,color:#fff
    style GitHub fill:#24292e,color:#fff
    style GeminiAPI fill:#4285f4,color:#fff
    style K8sAPI fill:#326ce5,color:#fff
    
    classDef stable fill:#4caf50,color:#fff
    classDef canary fill:#ff9800,color:#fff
    classDef external fill:#9e9e9e,color:#fff
    
    class StablePods stable
    class CanaryPods canary
    class GitHub,GitHubAPI,GeminiAPI,GoogleSearch external
```
