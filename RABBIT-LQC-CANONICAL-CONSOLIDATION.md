# Rabbit Chain — consolidação do `consensus/lqc`

Versão: `1.0.2`

Este pacote:

- ativa `consensus/lqc` para todo genesis com `config.lqc`;
- mantém `consensus/lqcv2` apenas como implementação legada e inativa;
- corrige a resolução local para usar a fila do próximo bloco;
- valida corretamente pais presentes no mesmo lote de sincronização;
- preserva o timestamp real do produtor depois do limite mínimo do slot;
- impede que um genesis com timestamp zero expire todas as janelas de fallback;
- lê `eth.blockNumber` como o número decimal nativo retornado pelo console;
- rejeita respostas RPC inválidas em vez de aceitá-las como hash de checkpoint;
- só declara o laboratório pronto após validar peers, alturas, hash comum e diversidade;
- inclui um verificador independente que audita o laboratório atual sem reiniciá-lo;
- preserva a recuperação de fork já aprovada;
- faz os auditores usarem `fallbackCount` e `committeeSize` configurados;
- mantém o primeiro halving exatamente no bloco `8.409.600`;
- alinha `RabbitChainConfig`, `RabbitDevnetChainConfig` e o genesis Rabbit mainnet a essa fronteira.

Não reutilize o banco de dados produzido com `lqcv2` para validar esta versão. Depois que a validação local passar, inicie um laboratório novo e limpo com o script de 20 produtores.
