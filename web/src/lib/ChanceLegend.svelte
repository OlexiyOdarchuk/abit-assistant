<div class="legend">
  <span class="item"><i style="--c: var(--safety)"></i> Високий — проходиш у межах вільних місць</span>
  <span class="item"><i style="--c: var(--match)"></i> Середній — на межі, залежить від відмов</span>
  <span class="item"><i style="--c: var(--reach)"></i> Низький — місць менше за твій ранг</span>
</div>
<details class="how">
  <summary>Як ми рахуємо шанси</summary>
  <p>
    Беремо твій конкурсний бал (НМТ × коефіцієнти спеціальності, з регіональним
    коефіцієнтом області ЗВО) і твій ранг серед <b>реальних</b> конкурентів —
    тих, хто подав оригінали й справді бореться за бюджет. Порівнюємо ранг із
    кількістю вільних бюджетних місць після вже зарахованих. Кнопка
    «🔮 Хто піде деінде» уточнює: знімає тих, хто проходить на свій вищий
    пріоритет в іншому ЗВО.
  </p>
</details>

<style>
  .legend { display: flex; flex-wrap: wrap; gap: 0.4rem 1rem; margin: 0.3rem 0; }
  .item { display: inline-flex; align-items: center; gap: 0.4rem; color: var(--muted); font-size: 0.83rem; }
  .item i { width: 0.65rem; height: 0.65rem; border-radius: 3px; background: var(--c); flex: none; }
  .how { margin-top: 0.6rem; }
  .how summary { cursor: pointer; color: var(--accent); font-weight: 600; font-size: 0.9rem; }
  .how p { color: var(--muted); font-size: 0.88rem; margin: 0.5rem 0 0; }
</style>
