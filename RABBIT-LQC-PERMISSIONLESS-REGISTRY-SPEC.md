# Rabbit Chain — cadastro permissionless canônico

Versão da especificação: `1.3.0-draft`

Status: engine, pool, RPC e gossip implementados atrás de ativação explícita por
bloco; desabilitados por padrão e ainda não habilitados no genesis do laboratório.

## Objetivos

- qualquer pessoa pode solicitar entrada sem possuir ou bloquear RAB;
- nenhuma carteira administradora aprova ou remove participantes;
- todos os nós derivam a mesma fila a partir da cadeia canônica;
- um nó novo reconstrói o cadastro somente pelos headers;
- registro, heartbeat e saída são verificáveis sem contrato inteligente;
- todos os participantes ativos continuam com peso igual na fila.

## Registro

O candidato cria uma operação `REGISTER` contendo versão, endereço, sequência,
validade e nonce de prova. Ele procura um nonce cujo hash Keccak-256 esteja
abaixo do alvo definido por `proofDifficulty` e assina a operação com a própria
chave secp256k1.

A prova serve somente para limitar spam de cadastros. Ela não aumenta peso,
prioridade ou recompensa. Não existe depósito de tokens.

## Inclusão canônica

As operações serão propagadas por um pool LQC próprio, sem taxa e sem EVM. Um
produtor ativo inclui um conjunto limitado de operações no `Extra` do header.
O header também contém o `registryRoot` resultante. Todos os nós:

1. verificam assinatura, sequência, validade e LightHash;
2. aplicam as operações sobre o snapshot do header pai;
3. calculam novamente o `registryRoot`;
4. rejeitam o bloco se qualquer byte divergir.

Uma entrada incluída no bloco `N` participa da seleção somente a partir de
`N + 1 + activationDelay`. Assim, o produtor do próprio bloco nunca se autoriza.

## Formato canônico do header

O envelope binário começa com os quatro bytes `LQC\\x00`, seguidos por RLP
canônico contendo:

1. versão do envelope (`2`);
2. número do bloco;
3. `registryRoot` pós-bloco;
4. lista de operações assinadas.

O codec limita o `Extra` a 16 KiB e aceita no máximo 64 operações por bloco.
As operações são ordenadas por endereço e sequência, com critérios de desempate
determinísticos. Um par endereço/sequência duplicado, raiz zero, assinatura com
tamanho incorreto, ordem não canônica, RLP malformado ou versão desconhecida é
rejeitado. A validação criptográfica usa o `chainId`, a altura e a dificuldade
LightHash. A conferência do `registryRoot` é executada pela camada de snapshots
antes de um header ser aceito pela engine.

O pool em memória aceita no máximo 4.096 operações e mantém uma operação
pendente por endereço. Uma sequência maior só substitui a anterior depois de
ser válida contra o snapshot canônico atual. Operações expiradas são podadas;
operações já incluídas podem permanecer retidas até expirar para tolerar reorg,
mas nunca voltam a um header sem serem revalidadas contra o novo pai.

O RPC `lqc_submitRegistryOperation` recebe somente operações já assinadas; o nó
não recebe chave privada e não assina pelo usuário. Consultas de status e
pendências usam o mesmo namespace. O protocolo P2P separado `lqcr/1` confere
versão, network ID e genesis antes de propagar lotes limitados. Cada peer mantém
um conjunto limitado de hashes conhecidos para cortar loops de gossip.

## Heartbeat e saída

- produzir um bloco atualiza o heartbeat automaticamente;
- quem ainda não produziu envia uma operação `HEARTBEAT` assinada;
- `EXIT` é assinado pelo próprio participante;
- inatividade após `heartbeatWindow + heartbeatGrace` remove automaticamente o
  participante da fila, sem apagar seu histórico;
- reentrada exige nova sequência e nova prova LightHash.

## Reconstrução, reorg e checkpoints

O cadastro é um snapshot derivado de headers. Snapshots são indexados pelo hash
do bloco, nunca apenas pela altura. Uma reorganização seleciona o snapshot do
novo pai. Checkpoints periódicos podem ser armazenados localmente para acelerar
sincronização, mas o cache local nunca é fonte de consenso.

Cada snapshot armazena altura, hash do bloco, raiz do cadastro e participantes
em ordem canônica. Ao aplicar um filho, o nó confere continuidade, produtor
elegível no snapshot pai, envelope, operações e raiz pós-bloco. Produzir o bloco
atualiza o heartbeat antes das operações. A reconstrução de um nó novo começa no
snapshot-base determinístico da transição e reaplica os headers canônicos.

## Ativação na engine

O campo `registryProtocolBlock` controla a transição. O valor zero mantém o
formato legado. Antes da altura configurada, a engine exige `LQC:1:N`; a partir
dela, exige o envelope binário e deriva seleção, committee e heartbeat pelo
snapshot do pai. A rota canônica não consulta o `RuntimeRegistry`, não exige
bond e não possui fallback que autorize automaticamente o coinbase do header.

Na fronteira de ativação, o snapshot-base é criado deterministicamente a partir
dos participantes de bootstrap configurados e do hash do pai. Checkpoints
locais completos são gravados somente a cada `epochLength`; os demais snapshots
recentes ficam em cache LRU limitado. Um cache ausente ou inválido é reconstruído
pelos headers.

Uma configuração ativada sem dificuldade LightHash ou sem bootstrap válido é
rejeitada. Depois que uma das alturas conflitantes já foi alcançada, um nó também
rejeita a troca local de `registryProtocolBlock`, evitando uma bifurcação causada
por alteração tardia do genesis/configuração.

## Limites obrigatórios

- tamanho máximo do `Extra`;
- máximo de operações por bloco;
- validade máxima futura de uma operação;
- uma operação por endereço e sequência;
- ordenação canônica das operações antes da codificação;
- domínio de assinatura inclui o `chainId`;
- dificuldade nunca pode ser zero.

## Genesis

Uma cadeia vazia não pode selecionar com segurança o primeiro produtor. Antes
da mainnet haverá uma cerimônia pública de genesis: candidatos publicam provas
LightHash, a lista é ordenada por regra verificável e inserida no genesis. Após
o bloco zero, o cadastro fica permanentemente aberto e nenhuma chave possui
poder administrativo.

## Fases de integração

1. núcleo matemático e testes isolados — concluído;
2. codec do envelope versionado e `registryRoot` — concluído, não ativado;
3. snapshots por hash e reconstrução por headers — concluído, não ativado;
4. integração controlada na engine — concluída atrás de ativação;
5. pool/gossip/RPC de operações — implementados atrás de ativação e em validação;
6. laboratório com bootstrap + produtor 21 desconhecido;
7. saída, expiração, retorno, reorg e sincronização de nó novo;
8. repetição de recompensas, resiliência, carga e fronteiras.
