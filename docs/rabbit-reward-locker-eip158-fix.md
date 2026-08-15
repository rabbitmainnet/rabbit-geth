# Rabbit Chain — correção do locker apagado pelo EIP-158

Data: 2026-08-05  
Evidência live: blocos 1–39 da cadeia reiniciada com o binário correto.  
Escopo da correção: `core/vesting`; nenhuma regra monetária foi modificada.

## Sintoma comprovado

O auditor esperava 46,8 RAB nos 39 blocos, mas observou zero em saldo líquido, locker,
original locked balance e índice de vesting. O executável do node1 era o binário novo e
continha `LQCV2 REWARD`, eliminando a hipótese de build antigo.

## Causa

O locker grava storage no endereço de sistema `0x0000000000000000000000000000000000001001`.
Esse endereço não existe no genesis e era criado com saldo zero, nonce zero e código vazio.

O devnet ativa EIP-158 no bloco zero. Para a limpeza de contas vazias, o geth considera
apenas nonce, saldo e código; storage não torna uma conta não vazia. Assim,
`IntermediateRoot(true)` apagava a conta de sistema e todo o storage recém-gravado ao fim de
cada bloco.

## Correção mínima

Antes da primeira inclusão no índice de vesting, o código garante nonce interno `1` para a
conta de sistema. Nonce não cria RAB e não muda reward, supply, divisão 70/30, fila ou
cronograma. Ele apenas impede que a limpeza EIP-158 classifique a conta como vazia.

O teste `TestLockedRewardSurvivesEIP158Finalization` credita 1,2 RAB no bloco 1, executa
`IntermediateRoot(true)` e exige que locked balance, original balance, índice e destinatário
continuem presentes.

## Próxima validação

Compilar o cliente, reiniciar o laboratório temporário e executar o auditor desde o bloco 1.
O resultado esperado por bloco é 0,84 RAB bloqueado para o producer e 0,072 RAB bloqueado
para cada um dos cinco membros do committee, totalizando exatamente 1,2 RAB.
