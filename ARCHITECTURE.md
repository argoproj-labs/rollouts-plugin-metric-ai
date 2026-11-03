## Canary Analysis Process Flow

This diagram shows the complete flow from canary deployment through tiered AI analysis to automated PR creation:

```mermaid
flowchart TD
    Start([Argo Rollouts Canary Deployment]) --> Canary[Deploy Canary v2<br/>alongside Stable v1]
    Canary --> Traffic[Route Traffic<br/>10% to Canary<br/>90% to Stable]
    Traffic --> Analysis[Argo Rollouts Analysis<br/>runs Analysis Template]
    
    Analysis --> Plugin{Metric AI Plugin<br/>argoproj-labs/metric-ai}
    
    Plugin --> AgentCall[Single HTTP call:<br/>POST /analyze<br/>with pod selectors]
    
    AgentCall --> K8sAgent[Kubernetes AI Agent<br/>in cluster]
    
    K8sAgent --> AgentTools[Agent gathers evidence in parallel:<br/>rollout status ∥ canary metrics ∥<br/>pod logs and events]
    
    AgentTools --> CheapModel[Analysis tier 1:<br/>cheap LLM concludes<br/>promote or rollback]
    
    CheapModel --> Grader{Decision model grades:<br/>is the conclusion<br/>supported by the evidence?}
    
    Grader -->|Supported or neutral| Gate
    Grader -->|Clearly disputed| StrongModel[Analysis tier 2:<br/>stronger LLM re-analyzes<br/>the same evidence]
    StrongModel --> Gate{Promote with low<br/>effective confidence<br/>and gate enabled?}
    
    Gate -->|Yes| Pause[Pause rollout<br/>for human review]
    Gate -->|No| NeedsFix{Rollback with<br/>a repo configured?}
    
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
    Pause --> Human[Human reviews<br/>promote or abort]
    Human --> Promote
    Human --> Abort
    
    style Start fill:#e1f5e1
    style Complete fill:#e1f5e1
    style K8sAgent fill:#fff3cd
    style Plugin fill:#d1ecf1
    style Grader fill:#f0e6ff
    style StrongModel fill:#f0e6ff
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
            AgentPod[kubernetes-agent<br/>parallel data agents, analysis,<br/>grader, escalation, pause gate]
            RemediationSvc[remediation-agent<br/>A2A remediation service]
            Vault[Vault + Secrets Operator<br/>credential sync]
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
        CheapLLM[Analysis LLM<br/>open weights, e.g. Qwen]
        StrongLLM[Escalation LLM<br/>open weights, e.g. DeepSeek]
        DecisionModel[Decision Model<br/>Jev via TypeSafe]
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
    ArgoController -->|POST /analyze| AgentService
    
    %% Agent Operations
    AgentService --> AgentPod
    AgentPod -->|queries, pauses| K8sAPI
    AgentPod -->|get logs| StablePods
    AgentPod -->|get logs| CanaryPods
    AgentPod -->|get metrics| MetricsServer
    AgentPod -->|tier 1 analysis| CheapLLM
    AgentPod -->|grades conclusions| DecisionModel
    AgentPod -->|tier 2 on dispute| StrongLLM
    AgentPod -->|async A2A after rollback| RemediationSvc
    
    %% Credential sync
    Vault -->|secrets| AgentPod
    Vault -->|secrets| RemediationSvc
    
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
    style DecisionModel fill:#7b1fa2,color:#fff
    style GitHub fill:#24292e,color:#fff
    style CheapLLM fill:#4285f4,color:#fff
    style StrongLLM fill:#4285f4,color:#fff
    style K8sAPI fill:#326ce5,color:#fff
    style Vault fill:#ffecb3
    
    classDef stable fill:#4caf50,color:#fff
    classDef canary fill:#ff9800,color:#fff
    classDef external fill:#9e9e9e,color:#fff
    
    class StablePods stable
    class CanaryPods canary
    class GitHub,CheapLLM,StrongLLM,DecisionModel external
```

## Analysis pipeline detail

Inside the kubernetes-agent, one analysis request runs this pipeline:

```mermaid
flowchart LR
    Req[POST /analyze] --> Data[Parallel data agents<br/>non-AI: rollout status,<br/>metrics, log diagnostics]
    Data --> Combine[Combine into<br/>diagnostic data]
    Combine --> T1[Analysis LLM<br/>cheap tier]
    T1 --> Grade{Jev decision model:<br/>conclusion supported?}
    Grade -->|clearly disputed| T2[Analysis LLM<br/>strong tier]
    T2 --> Grade2{Re-grade}
    Grade -->|supported / neutral| Decide
    Grade2 -->|supported / neutral| Decide
    Grade2 -->|still disputed| Low[Effective confidence<br/>drops to the grade]
    Low --> Decide{Gate}
    Decide -->|promote, high confidence| OK[Return promote]
    Decide -->|promote, low confidence<br/>and gate enabled| Pause[Pause for human]
    Decide -->|rollback| Roll[Return rollback<br/>+ delegate remediation]
    
    style T1 fill:#cfe2ff
    style T2 fill:#e3d7ff
    style Grade fill:#f0e6ff
    style Grade2 fill:#f0e6ff
    style Pause fill:#fff3cd
    style Roll fill:#f8d7da
```
