# Rabbit LQC — pesquisa de tickets sequenciais v1

Status: modelo matemático para pesquisa. Não implementado no consenso.

## Resultado do protótipo anterior

O benchmark `rabbit-lowend-accessibility-benchmark/1.0.2` separou execução contínua e operações isoladas. O uso atual de `argon2.IDKey` apresentou pausas severas de alocação/GC e instabilidade nas memórias maiores. Esse protótipo foi rejeitado; nenhum parâmetro foi selecionado.

## Requisito que permanece

Criar endereços adicionais sem trabalho adicional não pode aumentar producer, fallbacks ou committee. Um PC fraco deve conseguir produzir uma unidade válida de trabalho sem GPU, stake ou saldo de RAB.

Nenhum mecanismo permissionless consegue provar que duas chaves pertencem à mesma pessoa. A meta tecnicamente verificável é uma chance por unidade real de recurso, e não uma chance por ser humano.

## Modelo sequencial

Uma lane executa uma sequência vinculada ao desafio canônico e produz no máximo um ticket elegível por época. Dividir a mesma lane entre 1 ou 5.000 identidades não cria tickets adicionais. Para produzir em paralelo, o atacante precisa de lanes reais adicionais.

Uma futura implementação pode pesquisar uma Verifiable Delay Function. O [trabalho original sobre VDFs](https://eprint.iacr.org/2018/601) define funções que exigem uma quantidade de etapas sequenciais para produzir uma saída única, com verificação pública eficiente, e cita leader election como aplicação. Esta referência não constitui uma implementação aprovada.

## Limite fundamental

Trabalho sequencial neutraliza identidades gratuitas, mas não neutraliza recursos reais. Um atacante com 64 lanes contra 20 honestas controla aproximadamente 76% da seleção. Contra 1.000 lanes honestas, as mesmas 64 representam aproximadamente 6%.

Portanto, a segurança depende também de uma base honesta ampla. A mainnet não pode tratar 20 processos no mesmo computador como 20 participantes independentes.

## Alternativas examinadas

- **RandomX:** a implementação oficial informa 2.080 MiB para o modo rápido de mineração; o modo leve usa 256 MiB, mas é significativamente mais lento e destinado à verificação. Isso conflita com o perfil fraco de 4 GiB.
- **Cuckoo Cycle:** o projeto oficial descreve trabalho fortemente limitado por memória e verificação instantânea. Continua candidato de pesquisa, mas ainda precisa de benchmark de RAM, minerador e hardware especializado.
- **Argon2:** o RFC 9106 cobre aplicações de proof of work, porém a implementação Go medida não passou no gate de estabilidade. Uma implementação com memória reutilizável ou Argon2d exigiria novo código, vetores oficiais e revisão independente.
- **VDF/trabalho sequencial:** melhor alinhamento conceitual com uma lane simples e verificação eficiente, porém possui maior complexidade criptográfica e não será escrita do zero sem implementação auditada.

## Gates antes de qualquer integração

1. Simular identidades fixas e recursos crescentes.
2. Demonstrar custo acessível em PC fraco real.
3. Usar implementação criptográfica conhecida e vetores públicos.
4. Auditar unicidade, sequencialidade, replay, grinding e paralelização.
5. Limitar provas por bloco e custo de entradas inválidas.
6. Repetir ataque Sybil, committee capture, reorg e resiliência.
7. Executar testnet pública com computadores e operadores independentes.
8. Manter mainnet bloqueada até revisão externa.

