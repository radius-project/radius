using System;
using System.ComponentModel.DataAnnotations;

namespace ui.Pages
{
    public class Order
    {
        public string OrderId { get; set; } = Guid.NewGuid().ToString();

        [Required]
        public string Item { get; set; } 

        [Required]
        public decimal? Price { get; set; }
    }
}