# Rabbit LQC — proposta de tickets de trabalho contínuos v1

Status: especificação para simulação. Não implementada no consenso.

## Problema confirmado

O cadastro atual concede uma posição completa na seleção para cada endereço que conclui um LightHash de registro. A prova é paga apenas uma vez. Um único controlador pode criar milhares de chaves e manter milhares de posições na fila, nos fallbacks e no committee.

O auditor `rabbit-lqc-sybil-auditor/1.0.0` confirmou que 5.000 endereços, contra 20 honestos, recebem aproximadamente 99,6% da seleção.

## Objetivo de segurança

Criar endereços adicionais não pode gerar poder sem custo adicional. A participação deve ser proporcional a trabalho computacional recente e verificável.

Esta proposta não promete uma identidade por pessoa. Isso exigiria uma autoridade de identidade, KYC, stake, hardware confiável ou outro recurso externo. A garantia possível em uma rede permissionless é uma chance por unidade de trabalho.

## Ticket de trabalho

Um ticket é uma prova hash válida para uma época futura:

```text
ticketHash = Keccak256(
  "RABBIT-LQC-WORK-TICKET-V1" ||
  chainID ||
  eligibilityEpoch ||
  challenge ||
  signingKey ||
  payoutAddress ||
  nonce
)

ticketHash < target
```

Cada tentativa exige trabalho. Criar chaves ou carteiras sem encontrar hashes abaixo do target não cria tickets e não altera a seleção.

## Ciclo de vida por época

1. A época `N` começa com um desafio derivado de um checkpoint canônico já finalizado.
2. Mineradores procuram tickets durante uma janela pública de submissão.
3. Cada ticket é assinado e vinculado à chain ID, época, chave produtora e endereço de pagamento.
4. A janela fecha antes de existir a seed de seleção da época de uso.
5. A seed nasce de um checkpoint posterior ao fechamento.
6. Os tickets são usados somente na época indicada e expiram ao final dela.
7. A próxima participação exige trabalho novo.

Uma ativação atrasada de duas épocas separa o trabalho da seed de seleção e reduz grinding:

```text
época N:     minerar e publicar tickets
época N+1:   fechamento, confirmação e seed futura
época N+2:   tickets elegíveis para producer, fallbacks e committee
```

## Seleção LCQ

A fila deixa de ordenar endereços registrados e passa a ordenar identificadores de tickets válidos:

```text
score = Keccak256(selectionSeed || ticketHash)
```

Os menores scores ocupam, em ordem:

1. producer;
2. fallbacks;
3. committee.

Um controlador pode usar várias chaves, mas cada posição adicional exige outro ticket válido. Dividir a mesma quantidade de tentativas entre mil chaves não altera o número esperado de tickets.

## Experiência do usuário

O usuário mantém uma única carteira de pagamento. O cliente Rabbit deve criar e renovar tickets automaticamente em segundo plano, sem exigir que a pessoa administre milhares de chaves, nonces ou operações RPC. Chaves efêmeras de produção podem ser vinculadas ao mesmo payout por assinatura, mas nunca podem criar peso sem a prova correspondente.

A tela do minerador deve mostrar pelo menos: época atual, desafio, taxa de tentativas, tickets encontrados, validade, chance estimada e confirmação canônica. Participar não exige saldo de RAB, stake ou autorização administrativa.

## Papel de IP e dispositivo

IP, ASN, fingerprint, MAC e identificadores de hardware não participam do cálculo de consenso. Eles podem limitar conexões ou mensagens no transporte P2P, mas nunca podem decidir ticket, producer, fallback, committee ou recompensa. Isso evita prejudicar usuários sob NAT/CGNAT e evita entregar poder a provedores, VPNs, nuvens ou fabricantes.

## Dificuldade e capacidade

A dificuldade deve ser determinística e usar somente dados canônicos de épocas anteriores. O ajuste precisa:

- mirar uma quantidade limitada de tickets por época;
- ter limites mínimos e máximos;
- reagir com atraso para evitar manipulação instantânea;
- usar aritmética inteira idêntica em todos os clientes;
- ser coberto por testes de fronteira e overflow.

O protocolo também precisa definir limites para operações por bloco, tickets por época, tamanho de mensagens e cache. Quando a procura ultrapassar a capacidade, a dificuldade deve reduzir a taxa de tickets, em vez de depender da ordem de chegada RPC.

## Estado canônico

O estado mínimo de um ticket contém:

- versão;
- chain ID;
- época de elegibilidade;
- challenge;
- ticket hash;
- chave de assinatura;
- payout;
- assinatura;
- bloco canônico de inclusão.

Tickets devem ser reconstruíveis pelos headers, isolados por hash de bloco, revertidos em reorgs e removidos após expiração. Cache local nunca pode ser fonte de consenso.

## Bootstrap

Participantes bootstrap podem ter uma janela curta de inicialização, definida antes do lançamento. Depois dela, todos, inclusive bootstraps, precisam de tickets. Não pode existir privilégio permanente.

## Recompensas

O ticket selecionado define a chave autorizada a assinar o bloco e o payout da recompensa. A divisão 70/30, a emissão, o halving e a recompensa imediata não precisam mudar por causa desta proposta.

## Ataques que precisam de testes dedicados

- multiplicação de 100, 1.000, 5.000 e 100.000 chaves com trabalho fixo;
- aumento real de capacidade computacional do adversário;
- grinding de chaves, payout, nonce, timestamp e checkpoint;
- retenção estratégica de tickets;
- flood do pool e gossip;
- tickets duplicados, replay entre épocas e redes;
- reorg durante submissão, fechamento e ativação;
- queda parcial e reinício total;
- produtor e committee formados por tickets do mesmo controlador;
- ajuste de dificuldade sob crescimento e perda brusca de hashpower;
- sincronização de nó novo e reconstrução histórica;
- consumo de CPU, memória, disco e largura de banda.

## Gates obrigatórios

1. O simulador deve mostrar que identidades adicionais, com trabalho total constante, não aumentam producer, fallbacks ou committee.
2. A implementação deve passar testes unitários e fuzzing.
3. Um laboratório limpo deve repetir o cenário de milhares de chaves.
4. Recompensas, assinaturas, transações, resiliência e fronteiras devem continuar em PASS.
5. Somente depois desses gates a mainnet pode ser reconsiderada.
